package ls

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

// StockMaster is one t8436 stock master row.
type StockMaster struct {
	Name       string
	Symbol     string
	MarketCode string
	ETFType    string
	UpperLimit float64
	LowerLimit float64
	PrevClose  float64
}

// OverseasStockMaster is one g3190 overseas stock master row.
type OverseasStockMaster struct {
	KeySymbol string
	Nation    string
	Exchange  string
	Symbol    string
	ISIN      string
	Name      string
	NameEn    string
	Currency  string
	Suspended bool
}

// RealtimeSubscription identifies one LS realtime registration request.
type RealtimeSubscription struct {
	TRCode string
	TRKey  string
}

// InquirePrice returns raw t1102 response fields for a stock.
func (c *Client) InquirePrice(ctx context.Context, symbol, exchange string) (map[string]interface{}, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	exchange = normalizeExchangeInput(exchange)
	if exchange == "" {
		exchange = "K"
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathStockMarket, TRStockQuote, map[string]interface{}{
		"t1102InBlock": map[string]interface{}{
			"shcode":    symbol,
			"exchgubun": exchange,
		},
	})
	if err != nil {
		return nil, err
	}
	block, ok := mapValue(resp, "t1102OutBlock")
	if !ok {
		return nil, fmt.Errorf("%w: t1102OutBlock missing", broker.ErrServerError)
	}
	return block, nil
}

// InquireChart fetches t8410 day/week/month chart rows.
func (c *Client) InquireChart(ctx context.Context, symbol, interval string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	gubun, err := chartGubun(interval)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	toDate := time.Now().Format("20060102")
	if !to.IsZero() {
		toDate = to.Format("20060102")
	}
	fromDate := ""
	if !from.IsZero() {
		fromDate = from.Format("20060102")
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathStockChart, TRStockChart, map[string]interface{}{
		"t8410InBlock": map[string]interface{}{
			"shcode":   symbol,
			"gubun":    gubun,
			"qrycnt":   limit,
			"sdate":    fromDate,
			"edate":    toDate,
			"cts_date": "",
			"comp_yn":  "N",
			"sujung":   "Y",
		},
	})
	if err != nil {
		return nil, err
	}
	return sliceValue(resp, "t8410OutBlock1"), nil
}

// InquireOverseasPrice returns raw g3101 response fields for an overseas stock.
func (c *Client) InquireOverseasPrice(ctx context.Context, symbol, exchange string) (map[string]interface{}, error) {
	symbol, exchange, keySymbol, err := normalizeOverseasRequestSymbol(symbol, exchange)
	if err != nil {
		return nil, err
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathOverseasStockMarket, TROverseasStockQuote, map[string]interface{}{
		"g3101InBlock": map[string]interface{}{
			"delaygb":   "R",
			"keysymbol": keySymbol,
			"exchcd":    exchange,
			"symbol":    symbol,
		},
	})
	if err != nil {
		return nil, err
	}
	block, ok := mapValue(resp, "g3101OutBlock")
	if !ok {
		return nil, fmt.Errorf("%w: g3101OutBlock missing", broker.ErrServerError)
	}
	return block, nil
}

// InquireOverseasInstrument returns raw g3104 response fields for an overseas stock.
func (c *Client) InquireOverseasInstrument(ctx context.Context, symbol, exchange string) (map[string]interface{}, error) {
	symbol, exchange, keySymbol, err := normalizeOverseasRequestSymbol(symbol, exchange)
	if err != nil {
		return nil, err
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathOverseasStockMarket, TROverseasStockInstrument, map[string]interface{}{
		"g3104InBlock": map[string]interface{}{
			"delaygb":   "R",
			"keysymbol": keySymbol,
			"exchcd":    exchange,
			"symbol":    symbol,
		},
	})
	if err != nil {
		return nil, err
	}
	block, ok := mapValue(resp, "g3104OutBlock")
	if !ok {
		return nil, fmt.Errorf("%w: g3104OutBlock missing", broker.ErrServerError)
	}
	return block, nil
}

// InquireOverseasChart fetches g3204 day/week/month/year chart rows.
func (c *Client) InquireOverseasChart(ctx context.Context, symbol, exchange, interval string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	symbol, exchange, keySymbol, err := normalizeOverseasRequestSymbol(symbol, exchange)
	if err != nil {
		return nil, err
	}
	gubun, err := overseasChartGubun(interval)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 5
	}
	// LS documents g3204 non-compressed responses as max 5 rows. Keep the
	// adapter on JSON row output until compressed chart payloads are mapped.
	if limit > 5 {
		limit = 5
	}
	toDate := ""
	if !to.IsZero() {
		toDate = to.Format("20060102")
	}
	fromDate := time.Now().AddDate(0, -1, 0).Format("20060102")
	if !from.IsZero() {
		fromDate = from.Format("20060102")
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathOverseasStockChart, TROverseasStockChart, map[string]interface{}{
		"g3204InBlock": map[string]interface{}{
			"delaygb":   "R",
			"keysymbol": keySymbol,
			"exchcd":    exchange,
			"symbol":    symbol,
			"gubun":     gubun,
			"qrycnt":    limit,
			"comp_yn":   "N",
			"sdate":     fromDate,
			"edate":     toDate,
			"cts_date":  "",
			"cts_info":  "",
			"sujung":    "Y",
		},
	})
	if err != nil {
		return nil, err
	}
	rows, ok := sliceValueOK(resp, "g3204OutBlock1")
	if !ok {
		msg := anyString(resp["rsp_msg"])
		if msg == "" {
			msg = "output block missing"
		}
		return nil, fmt.Errorf("%w: g3204OutBlock1 missing: %s", broker.ErrServerError, msg)
	}
	return rows, nil
}

// InquireBalance fetches t0424 account summary and position rows.
func (c *Client) InquireBalance(ctx context.Context) (map[string]interface{}, []map[string]interface{}, error) {
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathStockAccount, TRStockBalance, map[string]interface{}{
		"t0424InBlock": map[string]interface{}{
			"prcgb":       "1",
			"chegb":       "2",
			"dangb":       "0",
			"charge":      "1",
			"cts_expcode": "",
		},
	})
	if err != nil {
		return nil, nil, err
	}
	block, _ := mapValue(resp, "t0424OutBlock")
	return block, sliceValue(resp, "t0424OutBlock1"), nil
}

// ListStockMaster fetches LS stock master rows with t8436.
func (c *Client) ListStockMaster(ctx context.Context, gubun string) ([]StockMaster, error) {
	gubun = strings.TrimSpace(gubun)
	if gubun == "" {
		gubun = "0"
	}
	resp, err := c.CallEndpoint(ctx, httpMethodPost, PathStockMisc, TRStockMaster, map[string]interface{}{
		"t8436InBlock": map[string]interface{}{
			"gubun": gubun,
		},
	})
	if err != nil {
		return nil, err
	}
	rows := sliceValue(resp, "t8436OutBlock")
	out := make([]StockMaster, 0, len(rows))
	for _, row := range rows {
		symbol := normalizeSymbol(anyString(row["shcode"]))
		if symbol == "" {
			continue
		}
		out = append(out, StockMaster{
			Name:       anyString(row["hname"]),
			Symbol:     symbol,
			MarketCode: strings.TrimSpace(anyString(row["gubun"])),
			ETFType:    strings.TrimSpace(anyString(row["etfgubun"])),
			UpperLimit: anyFloat(row["uplmtprice"]),
			LowerLimit: anyFloat(row["dnlmtprice"]),
			PrevClose:  anyFloat(row["jnilclose"]),
		})
	}
	return out, nil
}

// ListOverseasStockMaster fetches LS overseas stock master rows with g3190.
func (c *Client) ListOverseasStockMaster(ctx context.Context, nation, exchangeGroup string, maxRows int) ([]OverseasStockMaster, error) {
	nation = strings.ToUpper(strings.TrimSpace(nation))
	if nation == "" {
		nation = "US"
	}
	exchangeGroup = strings.TrimSpace(exchangeGroup)
	if exchangeGroup == "" {
		exchangeGroup = "2"
	}
	readCount := 100
	if maxRows > 0 && maxRows < readCount {
		readCount = maxRows
	}

	out := make([]OverseasStockMaster, 0)
	cts := ""
	for {
		resp, err := c.CallEndpoint(ctx, httpMethodPost, PathOverseasStockMarket, TROverseasStockMaster, map[string]interface{}{
			"g3190InBlock": map[string]interface{}{
				"delaygb":   "R",
				"natcode":   nation,
				"exgubun":   exchangeGroup,
				"readcnt":   readCount,
				"cts_value": cts,
			},
		})
		if err != nil {
			return nil, err
		}
		rows := sliceValue(resp, "g3190OutBlock1")
		for _, row := range rows {
			symbol := normalizeOverseasSymbol(anyString(row["symbol"]))
			if symbol == "" {
				continue
			}
			out = append(out, OverseasStockMaster{
				KeySymbol: strings.TrimSpace(anyString(row["keysymbol"])),
				Nation:    strings.ToUpper(strings.TrimSpace(anyString(row["natcode"]))),
				Exchange:  strings.TrimSpace(anyString(row["exchcd"])),
				Symbol:    symbol,
				ISIN:      strings.TrimSpace(anyString(row["isin"])),
				Name:      anyString(row["korname"]),
				NameEn:    anyString(row["engname"]),
				Currency:  strings.ToUpper(strings.TrimSpace(anyString(row["currency"]))),
				Suspended: strings.EqualFold(strings.TrimSpace(anyString(row["suspend"])), "Y"),
			})
			if maxRows > 0 && len(out) >= maxRows {
				return out, nil
			}
		}
		block, _ := mapValue(resp, "g3190OutBlock")
		next := strings.TrimSpace(anyString(block["cts_value"]))
		if next == "" || next == cts || len(rows) < readCount {
			break
		}
		cts = next
	}
	return out, nil
}

// BuildTradeSubscriptions builds S3_/K3_ subscriptions from t8436 stock master rows.
func (c *Client) BuildTradeSubscriptions(ctx context.Context) ([]RealtimeSubscription, error) {
	rows, err := c.ListStockMaster(ctx, "0")
	if err != nil {
		return nil, err
	}
	out := make([]RealtimeSubscription, 0, len(rows))
	for _, row := range rows {
		trCD := ""
		switch strings.TrimSpace(row.MarketCode) {
		case "1":
			trCD = TRRealtimeKOSPI
		case "2":
			trCD = TRRealtimeKOSDAQ
		default:
			continue
		}
		out = append(out, RealtimeSubscription{TRCode: trCD, TRKey: row.Symbol})
	}
	return out, nil
}

// BuildOverseasTradeSubscriptions builds GSC subscriptions from g3190 overseas master rows.
func (c *Client) BuildOverseasTradeSubscriptions(ctx context.Context, nation, exchangeGroup string, maxRows int) ([]RealtimeSubscription, error) {
	rows, err := c.ListOverseasStockMaster(ctx, nation, exchangeGroup, maxRows)
	if err != nil {
		return nil, err
	}
	out := make([]RealtimeSubscription, 0, len(rows))
	for _, row := range rows {
		key := row.KeySymbol
		if key == "" {
			key = row.Exchange + row.Symbol
		}
		out = append(out, RealtimeSubscription{TRCode: TRRealtimeOverseasTrade, TRKey: padOverseasRealtimeKey(key)})
	}
	return out, nil
}

// OverseasRealtimeKey returns the fixed-width realtime key used by GSC/GSH.
func OverseasRealtimeKey(exchange, symbol string) (string, error) {
	_, _, keySymbol, err := normalizeOverseasRequestSymbol(symbol, exchange)
	if err != nil {
		return "", err
	}
	return padOverseasRealtimeKey(keySymbol), nil
}

func chartGubun(interval string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "", "1d", "d", "day", "daily":
		return "2", nil
	case "1w", "w", "week", "weekly":
		return "3", nil
	case "1mo", "mo", "month", "monthly":
		return "4", nil
	default:
		return "", fmt.Errorf("unsupported interval for LS: %s", interval)
	}
}

func overseasChartGubun(interval string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "", "1d", "d", "day", "daily":
		return "2", nil
	case "1w", "w", "week", "weekly":
		return "3", nil
	case "1mo", "mo", "month", "monthly":
		return "4", nil
	case "1y", "y", "year", "yearly":
		return "5", nil
	default:
		return "", fmt.Errorf("unsupported overseas interval for LS: %s", interval)
	}
}

func normalizeSymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	symbol = strings.TrimPrefix(symbol, "A")
	return symbol
}

func normalizeOverseasSymbol(symbol string) string {
	return strings.TrimSpace(strings.ToUpper(symbol))
}

func normalizeOverseasRequestSymbol(symbol, exchange string) (string, string, string, error) {
	symbol = normalizeOverseasSymbol(symbol)
	exchange = strings.TrimSpace(exchange)
	if len(symbol) > 2 {
		prefix := symbol[:2]
		if prefix == "81" || prefix == "82" {
			if exchange == "" {
				exchange = prefix
			}
			symbol = strings.TrimSpace(symbol[2:])
		}
	}
	if symbol == "" {
		return "", "", "", broker.ErrInvalidSymbol
	}
	if exchange != "81" && exchange != "82" {
		return "", "", "", broker.ErrInvalidMarket
	}
	return symbol, exchange, exchange + symbol, nil
}

func normalizeExchangeInput(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "", "KRX", "KOSPI", "KOSDAQ", "K":
		return "K"
	case "NXT", "N":
		return "N"
	case "U", "UNIFIED", "ALL":
		return "U"
	default:
		return ""
	}
}

func padOverseasRealtimeKey(key string) string {
	key = strings.TrimSpace(strings.ToUpper(key))
	if len(key) >= 18 {
		return key[:18]
	}
	return key + strings.Repeat(" ", 18-len(key))
}

func mapValue(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	switch t := v.(type) {
	case map[string]interface{}:
		return t, true
	default:
		return nil, false
	}
}

func sliceValue(m map[string]interface{}, key string) []map[string]interface{} {
	rows, _ := sliceValueOK(m, key)
	return rows
}

func sliceValueOK(m map[string]interface{}, key string) ([]map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	switch t := v.(type) {
	case []map[string]interface{}:
		return t, true
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(t))
		for _, item := range t {
			if row, ok := item.(map[string]interface{}); ok {
				out = append(out, row)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func anyString(v interface{}) string {
	return strings.TrimSpace(asString(v))
}

func anyFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case jsonNumber:
		f, _ := strconv.ParseFloat(t.String(), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(t), ",", ""), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(fmt.Sprint(t)), ",", ""), 64)
		return f
	}
}

type jsonNumber interface {
	String() string
}

const httpMethodPost = "POST"
