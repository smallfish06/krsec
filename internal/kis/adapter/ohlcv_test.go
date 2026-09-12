package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/pkg/broker"
)

func TestOHLCVUsesMarketHistoryAndRequestedDates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		market, symbol, path, exchange string
		overseas                       bool
	}{
		{"US-NASDAQ", "AAPL", kis.PathOverseasPriceDailyPrice, "NAS", true},
		{"US-NYSE", "IBM", kis.PathOverseasPriceDailyPrice, "NYS", true},
		{"KOSDAQ", "035900", kis.PathDomesticStockInquireDailyItemChartPrice, "J", false},
		{"NXT", "005930", kis.PathDomesticStockInquireDailyItemChartPrice, "NX", false},
		{"SOR", "005930", kis.PathDomesticStockInquireDailyItemChartPrice, "UN", false},
	} {
		t.Run(tc.market, func(t *testing.T) {
			a := newStubAdapter(t)
			a.dispatcher.routes[tc.path] = newEndpointRoute([]string{http.MethodGet}, func(_ context.Context, _, _ string, fields map[string]string) (any, error) {
				if tc.overseas {
					if fields["EXCD"] != tc.exchange || fields["SYMB"] != tc.symbol || fields["BYMD"] != "20260911" || fields["GUBN"] != "0" || fields["MODP"] != "1" {
						t.Fatalf("overseas request=%v", fields)
					}
				} else if fields["FID_COND_MRKT_DIV_CODE"] != tc.exchange || fields["FID_INPUT_ISCD"] != tc.symbol || fields["FID_INPUT_DATE_1"] != "20260901" || fields["FID_INPUT_DATE_2"] != "20260911" {
					t.Fatalf("domestic request=%v", fields)
				}
				return historyFixture(tc.overseas, mustDate(t, "2026-09-11"), 1), nil
			})
			rows, err := a.GetOHLCV(context.Background(), tc.market, tc.symbol, broker.OHLCVOpts{From: mustDate(t, "2026-09-01"), To: mustDate(t, "2026-09-11"), Interval: "1d"})
			if err != nil || len(rows) != 1 || rows[0].Open != 100 || rows[0].Close != 104 || rows[0].Volume != 1000 {
				t.Fatalf("candles=%v err=%v", rows, err)
			}
		})
	}
}

func TestOHLCVFetchesBeyondOneHundredRows(t *testing.T) {
	t.Parallel()
	for _, overseas := range []bool{false, true} {
		t.Run(fmt.Sprint(overseas), func(t *testing.T) {
			a := newStubAdapter(t)
			to := mustDate(t, "2026-09-11")
			path, market, dateField := kis.PathDomesticStockInquireDailyItemChartPrice, "KRX", "FID_INPUT_DATE_2"
			if overseas {
				path, market, dateField = kis.PathOverseasPriceDailyPrice, "US-NASDAQ", "BYMD"
			}
			calls := 0
			a.dispatcher.routes[path] = newEndpointRoute([]string{http.MethodGet}, func(_ context.Context, _, _ string, fields map[string]string) (any, error) {
				want := to.AddDate(0, 0, -100*calls)
				if fields[dateField] != want.Format("20060102") {
					t.Fatalf("pagination request=%v expected end=%s", fields, want)
				}
				calls++
				return historyFixture(overseas, want, 100), nil
			})
			rows, err := a.GetOHLCV(context.Background(), market, "TEST", broker.OHLCVOpts{To: to, Limit: 150})
			if err != nil || len(rows) != 150 || calls != 2 {
				t.Fatalf("rows=%d calls=%d err=%v", len(rows), calls, err)
			}
			if !rows[149].Timestamp.Equal(to.AddDate(0, 0, -149)) {
				t.Fatalf("oldest=%s", rows[149].Timestamp)
			}
		})
	}
}

func TestOHLCVBoundedPaginationAndNoProgress(t *testing.T) {
	t.Parallel()
	for _, repeat := range []bool{false, true} {
		t.Run(fmt.Sprint(repeat), func(t *testing.T) {
			a := newStubAdapter(t)
			to := mustDate(t, "2026-09-11")
			calls := 0
			a.dispatcher.routes[kis.PathOverseasPriceDailyPrice] = newEndpointRoute([]string{http.MethodGet}, func(_ context.Context, _, _ string, fields map[string]string) (any, error) {
				end := to
				if !repeat {
					var err error
					end, err = time.Parse("20060102", fields["BYMD"])
					if err != nil {
						t.Fatal(err)
					}
				}
				calls++
				return historyFixture(true, end, 100), nil
			})
			rows, err := a.GetOHLCV(context.Background(), "US-NASDAQ", "AAPL", broker.OHLCVOpts{From: mustDate(t, "2000-01-01"), To: to})
			wantCalls, wantError := maxOHLCVPages, "exceeds"
			if repeat {
				wantCalls, wantError = 2, "no progress"
			}
			if err == nil || !strings.Contains(err.Error(), wantError) || rows != nil || calls != wantCalls {
				t.Fatalf("must not claim partial history: rows=%d calls=%d err=%v", len(rows), calls, err)
			}
		})
	}
}

func TestOHLCVMonthlyDefaultLimitFitsBoundAndCompletesOldestPeriod(t *testing.T) {
	t.Parallel()
	a := newStubAdapter(t)
	to := mustDate(t, "2026-09-11")
	calls := 0
	a.dispatcher.routes[kis.PathOverseasPriceDailyPrice] = newEndpointRoute([]string{http.MethodGet}, func(_ context.Context, _, _ string, fields map[string]string) (any, error) {
		end, err := time.Parse("20060102", fields["BYMD"])
		if err != nil {
			t.Fatal(err)
		}
		calls++
		return historyFixture(true, end, 100), nil
	})
	rows, err := a.GetOHLCV(context.Background(), "US-NASDAQ", "AAPL", broker.OHLCVOpts{To: to, Interval: "1mo", Limit: 100})
	if err != nil || len(rows) != 100 || calls > maxOHLCVPages {
		t.Fatalf("HTTP default monthly request: rows=%d calls=%d err=%v", len(rows), calls, err)
	}
	oldest := rows[len(rows)-1]
	daysInMonth := time.Date(oldest.Timestamp.Year(), oldest.Timestamp.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if oldest.Volume != int64(daysInMonth)*1000 {
		t.Fatalf("oldest period is partial: volume=%d expected=%d", oldest.Volume, daysInMonth*1000)
	}
}

func TestOHLCVRejectsInvalidInputBeforeDispatch(t *testing.T) {
	t.Parallel()
	a := newStubAdapter(t)
	for _, tc := range []struct {
		market string
		opts   broker.OHLCVOpts
	}{
		{"unknown", broker.OHLCVOpts{}},
		{"US-NASDAQ", broker.OHLCVOpts{Interval: "5m"}},
		{"KRX", broker.OHLCVOpts{From: mustDate(t, "2026-09-12"), To: mustDate(t, "2026-09-11")}},
	} {
		if _, err := a.GetOHLCV(context.Background(), tc.market, "TEST", tc.opts); err == nil || strings.Contains(err.Error(), "unsupported KIS endpoint") {
			t.Fatalf("input should fail before dispatch: market=%s err=%v", tc.market, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.GetOHLCV(ctx, "US-NASDAQ", "AAPL", broker.OHLCVOpts{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}

func historyFixture(overseas bool, end time.Time, count int) map[string]any {
	rows := make([]map[string]string, 0, count)
	for i := 0; i < count; i++ {
		date := end.AddDate(0, 0, -i).Format("20060102")
		if overseas {
			rows = append(rows, map[string]string{"xymd": date, "open": "100", "high": "105", "low": "99", "clos": "104", "tvol": "1000"})
		} else {
			rows = append(rows, map[string]string{"stck_bsop_date": date, "stck_oprc": "100", "stck_hgpr": "105", "stck_lwpr": "99", "stck_clpr": "104", "acml_vol": "1000"})
		}
	}
	return map[string]any{"rt_cd": "0", "output2": rows}
}
