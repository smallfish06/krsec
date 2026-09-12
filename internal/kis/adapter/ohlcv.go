package adapter

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/pkg/broker"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

// Allows the HTTP default of 100 monthly candles as well as 2,000 daily
// candles, while bounding upstream calls for very large range requests.
const maxOHLCVPages = 32

// GetOHLCV retrieves adjusted candles using each market's documented history API.
// Dates represent source trading dates. Range requests page backward, with an
// explicit error if the bounded history budget cannot satisfy the request.
func (a *Adapter) GetOHLCV(ctx context.Context, market, symbol string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	if _, err := applyOHLCVOptions(nil, opts); err != nil {
		return nil, err
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("OHLCV limit must not be negative")
	}
	if !opts.From.IsZero() && !opts.To.IsZero() && opts.From.After(opts.To) {
		return nil, fmt.Errorf("OHLCV from date must not be after to date")
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	exchange, overseas := toKISOverseasQuoteExchange(market)
	if !overseas {
		switch strings.ToUpper(strings.TrimSpace(market)) {
		case "", "KRX", "KOSPI", "KOSDAQ", "KONEX", "KNX":
			exchange = "J"
		case "NXT":
			exchange = "NX"
		case "SOR":
			exchange = "UN"
		default:
			return nil, fmt.Errorf("%w: unsupported OHLCV market %q", broker.ErrInvalidMarket, market)
		}
	}
	from := tradingDate(opts.From)
	to := tradingDate(opts.To)
	if to.IsZero() {
		to = tradingDate(time.Now())
	}
	if !from.IsZero() && from.After(to) {
		return nil, fmt.Errorf("OHLCV from date must not be after to date")
	}
	filterOpts := opts
	if from.IsZero() && filterOpts.Limit == 0 {
		filterOpts.Limit = 30
	}
	if from.IsZero() {
		from = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	interval := strings.ToLower(strings.TrimSpace(opts.Interval))
	grouped := interval != "" && interval != "1d" && interval != "d" && interval != "day" && interval != "daily"
	rows := make([]broker.OHLCV, 0, 100)
	seen := make(map[time.Time]struct{})
	cursor := to
	for page := 0; page < maxOHLCVPages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := a.getOHLCVPage(ctx, exchange, symbol, overseas, from, cursor)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return applyOHLCVOptions(rows, filterOpts)
		}
		oldest := cursor.AddDate(0, 0, 1)
		added := 0
		for _, row := range batch {
			if row.Timestamp.Before(oldest) {
				oldest = row.Timestamp
			}
			if row.Timestamp.After(cursor) || row.Timestamp.Before(from) {
				continue
			}
			if _, ok := seen[row.Timestamp]; ok {
				continue
			}
			seen[row.Timestamp] = struct{}{}
			rows = append(rows, row)
			added++
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Timestamp.After(rows[j].Timestamp) })
		if !oldest.After(from) {
			return applyOHLCVOptions(rows, filterOpts)
		}
		if added == 0 || oldest.After(cursor) {
			return nil, fmt.Errorf("KIS OHLCV pagination made no progress")
		}
		if len(batch) < 100 {
			return applyOHLCVOptions(rows, filterOpts)
		}
		if filterOpts.Limit > 0 {
			unlimited := filterOpts
			unlimited.Limit = 0
			filtered, err := applyOHLCVOptions(rows, unlimited)
			if err != nil {
				return nil, err
			}
			needed := filterOpts.Limit
			if grouped {
				// Fetch through one older period so the last returned week's/month's
				// opening price and volume cannot come from an incomplete page.
				needed++
			}
			if len(filtered) >= needed {
				return applyOHLCVOptions(rows, filterOpts)
			}
		}
		cursor = oldest.AddDate(0, 0, -1)
	}
	return nil, fmt.Errorf("KIS OHLCV history exceeds %d pages; narrow the date range or limit", maxOHLCVPages)
}

func tradingDate(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (a *Adapter) getOHLCVPage(ctx context.Context, exchange, symbol string, overseas bool, from, to time.Time) ([]broker.OHLCV, error) {
	rows := make([]broker.OHLCV, 0, 100)
	if overseas {
		resp, err := callEndpointDecoded[kisspecs.KISOverseasPriceV1QuotationsDailyprice](a, ctx, http.MethodGet, kis.PathOverseasPriceDailyPrice, "", kisspecs.KISOverseasPriceV1QuotationsDailypriceRequest{
			Auth: "", Excd: exchange, Symb: symbol, Gubn: "0", Bymd: to.Format("20060102"), Modp: "1",
		})
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Output2 {
			if strings.TrimSpace(item.Xymd) == "" {
				continue
			}
			row, err := parseOHLCVRow(item.Xymd, item.Open, item.High, item.Low, item.Clos, item.Tvol)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	resp, err := callEndpointDecoded[kisspecs.KISDomesticStockV1QuotationsInquireDailyItemchartprice](a, ctx, http.MethodGet, kis.PathDomesticStockInquireDailyItemChartPrice, "", kisspecs.KISDomesticStockV1QuotationsInquireDailyItemchartpriceRequest{
		FidCondMrktDivCode: exchange, FidInputIscd: symbol, FidInputDate1: from.Format("20060102"), FidInputDate2: to.Format("20060102"), FidPeriodDivCode: "D", FidOrgAdjPrc: "0",
	})
	if err != nil {
		return nil, err
	}
	for _, item := range resp.Output2 {
		if strings.TrimSpace(item.StckBsopDate) == "" {
			continue
		}
		row, err := parseOHLCVRow(item.StckBsopDate, item.StckOprc, item.StckHgpr, item.StckLwpr, item.StckClpr, item.AcmlVol)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseOHLCVRow(date, open, high, low, closePrice, volume string) (broker.OHLCV, error) {
	timestamp, err := time.Parse("20060102", strings.TrimSpace(date))
	if err != nil {
		return broker.OHLCV{}, fmt.Errorf("invalid KIS OHLCV trading date %q: %w", date, err)
	}
	row := broker.OHLCV{Timestamp: timestamp}
	row.Open, _ = strconv.ParseFloat(open, 64)
	row.High, _ = strconv.ParseFloat(high, 64)
	row.Low, _ = strconv.ParseFloat(low, 64)
	row.Close, _ = strconv.ParseFloat(closePrice, 64)
	row.Volume, _ = strconv.ParseInt(volume, 10, 64)
	return row, nil
}
