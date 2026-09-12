package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/smallfish06/krsec/internal/toss"
	"github.com/smallfish06/krsec/pkg/broker"
)

func normalizedTestAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == toss.PathOAuthToken {
			_ = json.NewEncoder(w).Encode(toss.TokenResponse{AccessToken: "test-token", TokenType: "Bearer", ExpiresIn: 3600})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	a := NewAdapterWithOptions(false, "test-account", "1", newMemoryTokenManager(), nil)
	a.Client().SetBaseURL(srv.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: t.Name(), AppSecret: "test-secret"}); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestFractionalExecutionPreservesExactQuantities(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		quantity, filled, remaining string
		filledInt, remainingInt     int64
	}{
		{"0.500000", "0.500000", "0", 0, 0},
		{"1.5", "0.6", "0.9", 0, 0},
		{"2.8", "1.5", "1.3", 1, 1},
		{"0.3", "0.2", "0.1", 0, 0},
		{"9007199254740993.1", "9007199254740992.9", "0.2", 9007199254740992, 0},
	} {
		t.Run(tc.quantity+"_"+tc.filled, func(t *testing.T) {
			t.Parallel()
			a := normalizedTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != toss.PathOrders+"/fractional" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"result": toss.Order{
					OrderID: "fractional", Symbol: "AAPL", Side: "SELL", Currency: "USD", Status: "FILLED",
					Quantity: tc.quantity, OrderedAt: "2026-09-11T22:31:00+09:00",
					Execution: toss.OrderExecution{FilledQuantity: tc.filled},
				}})
			})
			order, err := a.GetOrder(context.Background(), "fractional")
			if err != nil {
				t.Fatal(err)
			}
			if order.FilledQuantityDecimal != tc.filled || order.RemainingQtyDecimal != tc.remaining || order.FilledQuantity != tc.filledInt || order.RemainingQty != tc.remainingInt {
				t.Fatalf("quantities lost: %+v", order)
			}
			fills, err := a.GetOrderFills(context.Background(), "fractional")
			if err != nil {
				t.Fatal(err)
			}
			if len(fills) != 1 || fills[0].QuantityDecimal != tc.filled || fills[0].Quantity != tc.filledInt {
				t.Fatalf("fractional execution lost: %+v", fills)
			}
		})
	}
}

func TestOrderFillsEmptyOnlyWhenNoExecution(t *testing.T) {
	t.Parallel()
	for _, filled := range []string{"0", "invalid"} {
		t.Run(filled, func(t *testing.T) {
			t.Parallel()
			a := normalizedTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"result": toss.Order{Quantity: "1", Execution: toss.OrderExecution{FilledQuantity: filled}}})
			})
			fills, err := a.GetOrderFills(context.Background(), "order")
			if filled == "0" {
				if err != nil || len(fills) != 0 {
					t.Fatalf("zero fill: %+v, %v", fills, err)
				}
			} else if !errors.Is(err, broker.ErrServerError) {
				t.Fatalf("invalid quantity must fail instead of report no execution: %v", err)
			}
		})
	}
}

func TestQuoteUsesExchangeTradingDate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, currency, quoteTime, current, previous string
	}{
		{"KR weekend last trade", "KRW", "2026-09-11T15:30:00+09:00", "2026-09-11T00:00:00+09:00", "2026-09-10T00:00:00+09:00"},
		{"US KST Saturday", "USD", "2026-09-12T04:30:00+09:00", "2026-09-11T00:00:00-04:00", "2026-09-10T00:00:00-04:00"},
		{"US DST change", "USD", "2026-11-03T03:30:00+09:00", "2026-11-02T00:00:00-05:00", "2026-10-30T00:00:00-04:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			quote := &broker.Quote{Price: 110, Close: 110}
			applyQuoteCandles(quote, parseTime(tc.quoteTime), tc.currency, []toss.Candle{
				{Timestamp: tc.current, Currency: tc.currency, OpenPrice: "105", HighPrice: "112", LowPrice: "103", ClosePrice: "110", Volume: "99"},
				{Timestamp: tc.previous, Currency: tc.currency, ClosePrice: "100"},
			})
			if quote.PrevClose != 100 || quote.Change != 10 || math.Abs(quote.ChangeRate-10) > 1e-10 || quote.Open != 105 || quote.Volume != 99 {
				t.Fatalf("wrong session selected: %+v", quote)
			}
		})
	}
}

func TestQuoteDoesNotReusePreviousSessionOHLCWhenCurrentCandleMissing(t *testing.T) {
	t.Parallel()
	quote := &broker.Quote{Price: 110, Close: 110}
	applyQuoteCandles(quote, parseTime("2026-09-14T09:00:01+09:00"), "KRW", []toss.Candle{
		{Timestamp: "2026-09-11T00:00:00+09:00", Currency: "KRW", OpenPrice: "90", ClosePrice: "100"},
		{Timestamp: "2026-09-10T00:00:00+09:00", Currency: "KRW", ClosePrice: "95"},
	})
	if quote.PrevClose != 100 || quote.Change != 10 || quote.Open != 0 || quote.Close != 110 {
		t.Fatalf("stale OHLC or wrong previous close: %+v", quote)
	}
}

func TestQuoteWithoutProviderTimestampDoesNotGuessDailyChange(t *testing.T) {
	t.Parallel()
	a := normalizedTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != toss.PathPrices {
			t.Errorf("no reliable price session; unexpected candle request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": []toss.PriceResponse{{Symbol: "AAPL", LastPrice: "110", Currency: "USD"}}})
	})
	quote, err := a.GetQuote(context.Background(), "NASDAQ", "AAPL")
	if err != nil || quote == nil {
		t.Fatalf("quote=%+v error=%v", quote, err)
	}
	if quote.Price != 110 || quote.PrevClose != 0 || quote.Change != 0 {
		t.Fatalf("unexpected inferred change: %+v", quote)
	}
}

func TestBalanceKeepsBuyingPowerSeparateFromUnsupportedCashAndAssets(t *testing.T) {
	t.Parallel()
	a := normalizedTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case toss.PathHoldings:
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{
				"marketValue":         map[string]any{"amount": map[string]any{"krw": "100000", "usd": "200"}},
				"totalPurchaseAmount": map[string]any{"krw": "90000"},
				"profitLoss":          map[string]any{"amount": map[string]any{"krw": "10000"}, "rate": "0.1111"},
			}})
		case toss.PathBuyingPower:
			currency := r.URL.Query().Get("currency")
			amount := "30000"
			if currency == "USD" {
				amount = "50"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": toss.BuyingPowerResponse{Currency: currency, CashBuyingPower: amount}})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	})
	balance, err := a.GetBalance(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if balance.PositionValue != 100000 || balance.BuyingPower != 30000 || balance.BuyingPowerByCurrency["USD"] != 50 || balance.PositionValueByCurrency["USD"] != 200 {
		t.Fatalf("known fields lost: %+v", balance)
	}
	if balance.Cash != 0 || balance.TotalAssets != 0 || balance.WithdrawableCash != 0 || len(balance.CashByCurrency) != 0 || len(balance.TotalAssetsByCurrency) != 0 {
		t.Fatalf("unsupported fields inferred: %+v", balance)
	}
	for _, field := range []string{"cash", "cash_by_currency", "withdrawable_cash", "total_assets", "total_assets_by_currency"} {
		if !slices.Contains(balance.UnavailableFields, field) {
			t.Fatalf("missing unavailable marker %s: %+v", field, balance)
		}
	}
}

func TestBalancePropagatesBuyingPowerFailure(t *testing.T) {
	t.Parallel()
	for _, currency := range []string{"KRW", "USD"} {
		t.Run(currency, func(t *testing.T) {
			t.Parallel()
			a := normalizedTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == toss.PathHoldings {
					_, _ = fmt.Fprint(w, `{"result":{}}`)
					return
				}
				if r.URL.Query().Get("currency") == currency {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = fmt.Fprint(w, `{"error":{"code":"temporarily-unavailable","message":"retry later"}}`)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"result": toss.BuyingPowerResponse{Currency: r.URL.Query().Get("currency"), CashBuyingPower: "100"}})
			})
			balance, err := a.GetBalance(context.Background(), "")
			if balance != nil || !errors.Is(err, broker.ErrServerError) || !strings.Contains(err.Error(), currency) {
				t.Fatalf("partial failure hidden: balance=%+v error=%v", balance, err)
			}
		})
	}
}

func TestPlaceOrderPreservesRequestedFractionalQuantity(t *testing.T) {
	t.Parallel()
	a := normalizedTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != toss.PathOrders {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": toss.OrderResponse{OrderID: "fractional"}})
	})
	result, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Symbol: "AAPL", Market: "NASDAQ", Side: broker.OrderSideSell, Type: broker.OrderTypeMarket, QuantityDecimal: "0.500000"})
	if err != nil || result == nil {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if result.RemainingQtyDecimal != "0.500000" || result.RemainingQty != 0 {
		t.Fatalf("fractional request quantity lost: %+v", result)
	}
}
