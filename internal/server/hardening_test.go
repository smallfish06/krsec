package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

func TestAPIKey_RequiredOnEverythingButHealthAndDocs(t *testing.T) {
	t.Parallel()

	b := newMockBroker(t, "KIS")
	b.On("GetBalance", mock.Anything, "acc1").Return(&broker.Balance{AccountID: "acc1"}, nil)
	s := NewWithBrokersOptions("127.0.0.1", 18080,
		[]config.AccountConfig{{AccountID: "acc1"}},
		map[string]broker.Broker{"acc1": b},
		ServerOptions{APIKey: "s3cret"},
	)
	t.Cleanup(s.Close)

	cases := []struct {
		name   string
		path   string
		header [2]string
		want   int
	}{
		{"no key", "/accounts/acc1/balance", [2]string{}, http.StatusUnauthorized},
		{"wrong key", "/accounts/acc1/balance", [2]string{"Authorization", "Bearer nope"}, http.StatusUnauthorized},
		{"prefix of key", "/accounts/acc1/balance", [2]string{"X-API-Key", "s3cre"}, http.StatusUnauthorized},
		{"bearer", "/accounts/acc1/balance", [2]string{"Authorization", "Bearer s3cret"}, http.StatusOK},
		{"lowercase scheme", "/accounts/acc1/balance", [2]string{"Authorization", "bearer s3cret"}, http.StatusOK},
		{"x-api-key", "/accounts/acc1/balance", [2]string{"X-API-Key", "s3cret"}, http.StatusOK},
		{"health stays open", "/health", [2]string{}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.header[0] != "" {
				req.Header.Set(tc.header[0], tc.header[1])
			}
			rr := performFiberRequest(t, s, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestAPIKey_DisabledByDefault(t *testing.T) {
	t.Parallel()

	b := newMockBroker(t, "KIS")
	b.On("GetBalance", mock.Anything, "acc1").Return(&broker.Balance{AccountID: "acc1"}, nil)
	s := NewWithBrokers("127.0.0.1", 18080, []config.AccountConfig{{AccountID: "acc1"}}, map[string]broker.Broker{"acc1": b})
	t.Cleanup(s.Close)

	rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodGet, "/accounts/acc1/balance", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	t.Parallel()
	for addr, want := range map[string]bool{
		"localhost:8080": true,
		"127.0.0.1:8080": true,
		"[::1]:8080":     true,
		"0.0.0.0:8080":   false,
		"10.0.0.5:8080":  false,
		":8080":          false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

// A configured account must only ever authenticate with its own key pair:
// Authenticate stores credentials on the adapter every request shares.
func TestHandleAuthToken_ConfiguredAccountIgnoresForeignCredentials(t *testing.T) {
	t.Parallel()

	account := config.AccountConfig{AccountID: "kis-acc", Broker: "kis", AppKey: "real-key", AppSecret: "real-secret"}
	configured := broker.Credentials{AppKey: "real-key", AppSecret: "real-secret"}
	token := &broker.Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}

	cases := []struct {
		name string
		body string
		want int
	}{
		{"foreign credentials rejected", `{"broker":"kis","credentials":{"app_key":"attacker","app_secret":"x"}}`, http.StatusUnauthorized},
		{"right key wrong secret rejected", `{"broker":"kis","credentials":{"app_key":"real-key","app_secret":"x"}}`, http.StatusUnauthorized},
		{"omitted credentials use configured", `{"broker":"kis"}`, http.StatusOK},
		{"matching credentials accepted", `{"broker":"kis","credentials":{"app_key":"real-key","app_secret":"real-secret"}}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kis := newMockBroker(t, "KIS")
			if tc.want == http.StatusOK {
				kis.On("Authenticate", mock.Anything, configured).Return(token, nil).Once()
			}
			s := newOrderTestServer(map[string]broker.Broker{"kis-acc": kis}, []config.AccountConfig{account})

			rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodPost, "/auth/token", bytes.NewReader([]byte(tc.body))))
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				kis.AssertNotCalled(t, "Authenticate", mock.Anything, mock.Anything)
			}
		})
	}
}

func TestProxyAccountSelector(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body, query, want string
		wantErr           bool
	}{
		{"", "", "", false},
		{"", "acc-2", "acc-2", false},
		{"acc-1", "", "acc-1", false},
		{"12345678-01", "12345678", "12345678-01", false},
		{"acc-1", "acc-2", "", true},
	}
	for _, tc := range cases {
		got, err := proxyAccountSelector(tc.body, tc.query)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("proxyAccountSelector(%q, %q) = %q, %v; want %q, err=%v", tc.body, tc.query, got, err, tc.want, tc.wantErr)
		}
	}
}

type recordingKISCaller struct {
	*mockBrokerShim
	calls atomic.Int32
}

// mockBrokerShim satisfies broker.Broker for raw-proxy tests that only
// exercise CallEndpoint.
type mockBrokerShim struct{ broker.Broker }

func (m *mockBrokerShim) Name() string { return broker.NameKIS }

func (r *recordingKISCaller) CallEndpoint(context.Context, string, string, string, any) (any, error) {
	r.calls.Add(1)
	return map[string]any{"ok": true}, nil
}

// The static KIS routes document ?account_id=; it must select the account
// instead of silently falling back to the first KIS account.
func TestKISProxy_AccountIDQuerySelectsAccount(t *testing.T) {
	t.Parallel()

	first := &recordingKISCaller{mockBrokerShim: &mockBrokerShim{}}
	second := &recordingKISCaller{mockBrokerShim: &mockBrokerShim{}}
	s := newOrderTestServer(
		map[string]broker.Broker{"11111111-01": first, "22222222-01": second},
		[]config.AccountConfig{
			{AccountID: "11111111-01", Broker: "kis"},
			{AccountID: "22222222-01", Broker: "kis"},
		},
	)
	t.Cleanup(s.Close)

	const path = "/kis/domestic-stock/v1/trading/inquire-balance"
	post := func(target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		return performFiberRequest(t, s, req)
	}

	if rr := post(path+"?account_id=22222222-01", `{"tr_id":"TTTC8434R"}`); rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if first.calls.Load() != 0 || second.calls.Load() != 1 {
		t.Fatalf("query selector ignored: first=%d second=%d", first.calls.Load(), second.calls.Load())
	}

	rr := post(path+"?account_id=22222222-01", `{"tr_id":"TTTC8434R","account_id":"11111111-01"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("conflicting selectors: status = %d, want 400 body=%s", rr.Code, rr.Body.String())
	}
	if first.calls.Load() != 0 || second.calls.Load() != 1 {
		t.Fatalf("conflicting request reached a broker: first=%d second=%d", first.calls.Load(), second.calls.Load())
	}
}

func TestGroupPositions(t *testing.T) {
	t.Parallel()

	t.Run("merges lots in first-seen order", func(t *testing.T) {
		got := groupPositions([]broker.Position{
			{Symbol: "005930", AssetType: broker.AssetStock, Quantity: 10, AvgPrice: 100, ProfitLoss: 50, MarketValue: 1050, LoanDate: ""},
			{Symbol: "000660", AssetType: broker.AssetStock, Quantity: 1, AvgPrice: 500},
			{Symbol: "005930", AssetType: broker.AssetStock, Quantity: 30, AvgPrice: 200, ProfitLoss: -150, MarketValue: 5850, LoanDate: "20261101"},
		})
		if len(got) != 2 || got[0].Symbol != "005930" || got[1].Symbol != "000660" {
			t.Fatalf("unexpected grouping/order: %+v", got)
		}
		merged := got[0]
		if merged.Quantity != 40 || merged.AvgPrice != 175 || merged.ProfitLoss != -100 || merged.MarketValue != 6900 {
			t.Fatalf("unexpected merged row: %+v", merged)
		}
		if merged.LoanDate != "" {
			t.Fatalf("loan date of one lot leaked onto the merged row: %q", merged.LoanDate)
		}
	})

	// Fractional holdings carry Quantity 0; dividing by it produced NaN,
	// which made the whole positions response fail to encode.
	t.Run("fractional lots stay encodable", func(t *testing.T) {
		got := groupPositions([]broker.Position{
			{Symbol: "AAPL", AssetType: broker.AssetOverseas, QuantityDecimal: "0.25", AvgPrice: 200},
			{Symbol: "AAPL", AssetType: broker.AssetOverseas, QuantityDecimal: "0.5", AvgPrice: 260},
		})
		if len(got) != 1 || got[0].QuantityDecimal != "0.75" || got[0].AvgPrice != 240 {
			t.Fatalf("unexpected merged row: %+v", got)
		}
		if _, err := json.Marshal(got); err != nil {
			t.Fatalf("grouped positions do not encode: %v", err)
		}
	})

	t.Run("zero quantity lots stay encodable", func(t *testing.T) {
		got := groupPositions([]broker.Position{
			{Symbol: "X", AssetType: broker.AssetStock},
			{Symbol: "X", AssetType: broker.AssetStock},
		})
		if _, err := json.Marshal(got); err != nil {
			t.Fatalf("grouped positions do not encode: %v", err)
		}
	})
}

// One caller hanging up must not fail the other callers sharing its flight.
func TestKISProxyCache_FlightSurvivesInitiatorCancel(t *testing.T) {
	t.Parallel()

	cache := newKISProxyCache(KISProxyCacheOptions{})
	t.Cleanup(cache.stop)

	started := make(chan struct{})
	release := make(chan struct{})
	fn := func(ctx context.Context) (any, error) {
		close(started)
		<-release
		return "value", ctx.Err()
	}

	initiatorCtx, cancelInitiator := context.WithCancel(context.Background())
	initiatorErr := make(chan error, 1)
	go func() {
		_, err := cache.do(initiatorCtx, "k", fn)
		initiatorErr <- err
	}()
	<-started

	followerResult := make(chan any, 1)
	go func() {
		value, err := cache.do(context.Background(), "k", func(context.Context) (any, error) {
			return nil, errors.New("follower must join the running flight")
		})
		if err != nil {
			followerResult <- err
			return
		}
		followerResult <- value
	}()
	// Give the follower time to join before the initiator leaves.
	time.Sleep(50 * time.Millisecond)

	cancelInitiator()
	if err := <-initiatorErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("initiator err = %v, want context.Canceled", err)
	}
	close(release)

	if got := <-followerResult; got != "value" {
		t.Fatalf("follower got %v, want the shared flight's value", got)
	}
}

func TestAccountsSummary_NamesIncompleteAccounts(t *testing.T) {
	t.Parallel()

	good := newMockBroker(t, "KIS")
	good.On("GetBalance", mock.Anything, "good").Return(&broker.Balance{AccountID: "good", TotalAssets: 100}, nil).Once()
	bad := newMockBroker(t, "LS")
	bad.On("GetBalance", mock.Anything, "bad").Return((*broker.Balance)(nil), errors.New("upstream unavailable")).Once()
	s := newOrderTestServer(
		map[string]broker.Broker{"good": good, "bad": bad},
		[]config.AccountConfig{{AccountID: "good"}, {AccountID: "bad"}},
	)

	rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodGet, "/accounts/summary", nil))
	resp := decodeResponse(t, rr)
	want := "cannot aggregate balances: accounts unavailable or incomplete: bad"
	if rr.Code != http.StatusServiceUnavailable || resp.Error != want {
		t.Fatalf("status=%d error=%q, want 503 %q", rr.Code, resp.Error, want)
	}
}

func TestPlaceOrder_NormalizesAndValidatesSideAndType(t *testing.T) {
	t.Parallel()

	t.Run("case-insensitive side and type reach the broker canonicalized", func(t *testing.T) {
		b := newMockBroker(t, "KIS")
		b.On("PlaceOrder", mock.Anything, mock.MatchedBy(func(req broker.OrderRequest) bool {
			return req.Side == broker.OrderSideSell && req.Type == broker.OrderTypeLimit
		})).Return(&broker.OrderResult{OrderID: "1"}, nil).Once()
		s := newOrderTestServer(map[string]broker.Broker{"acc1": b}, []config.AccountConfig{{AccountID: "acc1"}})

		body := []byte(`{"symbol":"005930","market":"KRX","side":"SELL","type":" Limit ","quantity":1,"price":70000}`)
		rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodPost, "/accounts/acc1/orders", bytes.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
		}
	})

	for name, body := range map[string]string{
		"missing side":   `{"symbol":"005930","type":"limit","quantity":1,"price":70000}`,
		"unknown side":   `{"symbol":"005930","side":"short","type":"limit","quantity":1,"price":70000}`,
		"unknown type":   `{"symbol":"005930","side":"buy","type":"stop","quantity":1,"price":70000}`,
		"missing symbol": `{"side":"buy","type":"limit","quantity":1,"price":70000}`,
	} {
		t.Run(name, func(t *testing.T) {
			b := newMockBroker(t, "KIS")
			s := newOrderTestServer(map[string]broker.Broker{"acc1": b}, []config.AccountConfig{{AccountID: "acc1"}})
			rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodPost, "/accounts/acc1/orders", bytes.NewReader([]byte(body))))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 body=%s", rr.Code, rr.Body.String())
			}
			b.AssertNotCalled(t, "PlaceOrder", mock.Anything, mock.Anything)
		})
	}
}
