package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/smallfish06/krsec/internal/toss"
	"github.com/smallfish06/krsec/pkg/broker"
)

type memoryTokenManager struct {
	mu     sync.RWMutex
	tokens map[string]struct {
		token     string
		expiresAt time.Time
	}
}

func newMemoryTokenManager() *memoryTokenManager {
	return &memoryTokenManager{tokens: map[string]struct {
		token     string
		expiresAt time.Time
	}{}}
}

func (m *memoryTokenManager) GetToken(appKey string) (string, time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.tokens[appKey]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", time.Time{}, false
	}
	return entry.token, entry.expiresAt, true
}

func (m *memoryTokenManager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[appKey] = struct {
		token     string
		expiresAt time.Time
	}{token: token, expiresAt: expiresAt}
	return nil
}

func (m *memoryTokenManager) DeleteToken(appKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, appKey)
	return nil
}

func (m *memoryTokenManager) WaitForAuth(string) {}

func TestGetQuote_MapsPriceAndDailyCandle(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case toss.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(toss.TokenResponse{AccessToken: "token", TokenType: "Bearer", ExpiresIn: 3600})
		case toss.PathPrices:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{{
					"symbol": "005930", "timestamp": "2026-03-25T09:30:00+09:00", "lastPrice": "72000", "currency": "KRW",
				}},
			})
		case toss.PathCandles:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"candles": []map[string]any{{
						"timestamp": "2026-03-25T09:00:00+09:00",
						"openPrice": "71600", "highPrice": "72300", "lowPrice": "71500", "closePrice": "71800", "volume": "3521000", "currency": "KRW",
					}},
					"nextBefore": nil,
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	a := NewAdapterWithOptions(false, "toss-main", "1", newMemoryTokenManager(), nil)
	a.Client().SetBaseURL(srv.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "quote-client", AppSecret: "secret"}); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	quote, err := a.GetQuote(context.Background(), "KRX", "005930")
	if err != nil {
		t.Fatalf("GetQuote() error = %v", err)
	}
	if quote.Price != 72000 || quote.Open != 71600 || quote.Volume != 3521000 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}

func TestGetPositions_PreservesFractionalQuantity(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case toss.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(toss.TokenResponse{AccessToken: "token", TokenType: "Bearer", ExpiresIn: 3600})
		case toss.PathHoldings:
			if got := r.Header.Get("X-Tossinvest-Account"); got != "7" {
				t.Fatalf("account header = %q, want 7", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"totalPurchaseAmount": map[string]any{"krw": "0", "usd": "1553"},
					"marketValue": map[string]any{
						"amount":          map[string]any{"krw": "0", "usd": "1785"},
						"amountAfterCost": map[string]any{"krw": "0", "usd": "1771.43"},
					},
					"profitLoss": map[string]any{
						"amount": map[string]any{"krw": "0", "usd": "232"}, "amountAfterCost": map[string]any{"krw": "0", "usd": "218.43"}, "rate": "0.1494", "rateAfterCost": "0.1406",
					},
					"dailyProfitLoss": map[string]any{"amount": map[string]any{"krw": "0", "usd": "25"}, "rate": "0.0142"},
					"items": []map[string]any{{
						"symbol": "AAPL", "name": "Apple Inc.", "marketCountry": "US", "currency": "USD", "quantity": "5.5", "lastPrice": "178.5", "averagePurchasePrice": "155.3",
						"marketValue":     map[string]any{"purchaseAmount": "854.15", "amount": "981.75", "amountAfterCost": "979.5"},
						"profitLoss":      map[string]any{"amount": "127.6", "amountAfterCost": "125.35", "rate": "0.1494", "rateAfterCost": "0.1406"},
						"dailyProfitLoss": map[string]any{"amount": "25", "rate": "0.0142"},
						"cost":            map[string]any{"commission": "1.5", "tax": nil},
					}},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	a := NewAdapterWithOptions(false, "toss-main", "7", newMemoryTokenManager(), nil)
	a.Client().SetBaseURL(srv.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "positions-client", AppSecret: "secret"}); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	positions, err := a.GetPositions(context.Background(), "toss-main")
	if err != nil {
		t.Fatalf("GetPositions() error = %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions len = %d", len(positions))
	}
	if positions[0].Quantity != 5 || positions[0].QuantityDecimal != "5.5" {
		t.Fatalf("unexpected quantity fields: %+v", positions[0])
	}
}

func TestPlaceOrder_MapsCommonRequest(t *testing.T) {
	t.Parallel()

	var orderBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case toss.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(toss.TokenResponse{AccessToken: "token", TokenType: "Bearer", ExpiresIn: 3600})
		case toss.PathOrders:
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if got := r.Header.Get("X-Tossinvest-Account"); got != "1" {
				t.Fatalf("account header = %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&orderBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"orderId": "ord-1", "clientOrderId": "cid-1"}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	a := NewAdapterWithOptions(false, "toss-main", "1", newMemoryTokenManager(), nil)
	a.Client().SetBaseURL(srv.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "order-client", AppSecret: "secret"}); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	result, err := a.PlaceOrder(context.Background(), broker.OrderRequest{
		ClientOrderID: "cid-1",
		Symbol:        "005930",
		Side:          broker.OrderSideBuy,
		Type:          broker.OrderTypeLimit,
		Quantity:      10,
		Price:         70000,
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if result.OrderID != "ord-1" {
		t.Fatalf("order id = %q", result.OrderID)
	}
	if orderBody["clientOrderId"] != "cid-1" || orderBody["side"] != "BUY" || orderBody["orderType"] != "LIMIT" {
		t.Fatalf("unexpected order body: %#v", orderBody)
	}
	if orderBody["quantity"] != "10" || orderBody["price"] != "70000" {
		t.Fatalf("unexpected numeric body: %#v", orderBody)
	}
}
