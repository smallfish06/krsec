package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

type proxyStubBroker struct {
	name string
}

func (b *proxyStubBroker) Name() string { return b.name }

func (b *proxyStubBroker) Authenticate(context.Context, broker.Credentials) (*broker.Token, error) {
	return &broker.Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (b *proxyStubBroker) GetQuote(context.Context, string, string) (*broker.Quote, error) {
	return &broker.Quote{}, nil
}

func (b *proxyStubBroker) GetOHLCV(context.Context, string, string, broker.OHLCVOpts) ([]broker.OHLCV, error) {
	return []broker.OHLCV{}, nil
}

func (b *proxyStubBroker) GetBalance(context.Context, string) (*broker.Balance, error) {
	return &broker.Balance{}, nil
}

func (b *proxyStubBroker) GetPositions(context.Context, string) ([]broker.Position, error) {
	return []broker.Position{}, nil
}

func (b *proxyStubBroker) PlaceOrder(context.Context, broker.OrderRequest) (*broker.OrderResult, error) {
	return &broker.OrderResult{}, nil
}

func (b *proxyStubBroker) CancelOrder(context.Context, string) error { return nil }

func (b *proxyStubBroker) ModifyOrder(context.Context, string, broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	return &broker.OrderResult{}, nil
}

type proxyKISBroker struct {
	proxyStubBroker
	called    bool
	calls     int
	gotMethod string
	gotPath   string
	gotTRID   string
	gotFields map[string]string
	resp      map[string]interface{}
	err       error
}

func (b *proxyKISBroker) CallEndpoint(
	_ context.Context,
	method string,
	path string,
	trID string,
	request interface{},
) (interface{}, error) {
	b.called = true
	b.calls++
	b.gotMethod = method
	b.gotPath = path
	b.gotTRID = trID
	if request == nil {
		b.gotFields = nil
	} else if fields, ok := request.(map[string]string); ok {
		b.gotFields = fields
	} else {
		b.gotFields = map[string]string{}
	}
	return b.resp, b.err
}

func TestHandleKISProxy_CachesPublicQuoteResponse(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0", "price": "70000"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"tr_id":"FHKST01010100","params":{"FID_COND_MRKT_DIV_CODE":"J","FID_INPUT_ISCD":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/inquire-price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	kisBroker.resp = map[string]interface{}{"rt_cd": "0", "price": "1"}
	req = httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/inquire-price", bytes.NewReader(body))
	rr = performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected second 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.calls != 1 {
		t.Fatalf("broker calls = %d, want 1", kisBroker.calls)
	}
	resp := decodeResponse(t, rr)
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response data = %T, want map", resp.Data)
	}
	if got := data["price"]; got != "70000" {
		t.Fatalf("cached price = %v, want 70000", got)
	}
}

func TestHandleKISProxy_DoesNotCacheTradingEndpoints(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"tr_id":"TTTC8434R","params":{"CANO":"12345678","ACNT_PRDT_CD":"01"}}`)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/trading/inquire-balance", bytes.NewReader(body))
		rr := performFiberRequest(t, s, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
	}
	if kisBroker.calls != 2 {
		t.Fatalf("broker calls = %d, want 2", kisBroker.calls)
	}
}

func TestHandleKISProxy_RateLimitsOutboundCalls(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := NewWithBrokersOptions(
		"127.0.0.1",
		18080,
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
		map[string]broker.Broker{"kis-acc": kisBroker},
		ServerOptions{
			KISProxyRateLimit: KISProxyRateLimitOptions{
				RequestsPerSecond: 20,
				Burst:             1,
			},
		},
	)

	body := []byte(`{"tr_id":"TTTC8434R","params":{"CANO":"12345678","ACNT_PRDT_CD":"01"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/trading/inquire-balance", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	start := time.Now()
	req = httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/trading/inquire-balance", bytes.NewReader(body))
	rr = performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected second 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("second uncached call elapsed = %s, want at least 40ms", elapsed)
	}
	if kisBroker.calls != 2 {
		t.Fatalf("broker calls = %d, want 2", kisBroker.calls)
	}
}

func TestHandleKISProxy_ServesStaleCacheOnEndpointError(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0", "price": "70000"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)
	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	s.kisCache.now = func() time.Time { return now }

	body := []byte(`{"tr_id":"FHKST01010100","params":{"FID_COND_MRKT_DIV_CODE":"J","FID_INPUT_ISCD":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/inquire-price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	now = now.Add(10 * time.Minute)
	kisBroker.resp = nil
	kisBroker.err = errors.New("upstream unavailable")

	req = httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/inquire-price", bytes.NewReader(body))
	rr = performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stale 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.calls != 2 {
		t.Fatalf("broker calls = %d, want 2", kisBroker.calls)
	}
	resp := decodeResponse(t, rr)
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response data = %T, want map", resp.Data)
	}
	if got := data["price"]; got != "70000" {
		t.Fatalf("stale price = %v, want 70000", got)
	}
}

func TestHandleKISProxy_DefaultRouteAndFirstKISAccount(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0", "msg_cd": "MCA00000"},
	}
	kiwoomBroker := &proxyStubBroker{name: "KIWOOM"}

	s := newOrderTestServer(
		map[string]broker.Broker{
			"kiwoom-acc": kiwoomBroker,
			"kis-acc":    kisBroker,
		},
		[]config.AccountConfig{
			{AccountID: "kiwoom-acc", Broker: "kiwoom"},
			{AccountID: "kis-acc", Broker: "kis"},
		},
	)

	body := []byte(`{"tr_id":"HHDFS00000300","params":{"EXCD":"NAS","SYMB":"AAPL"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/overseas-price/v1/quotations/price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Broker != "KIS" {
		t.Fatalf("broker = %q, want KIS", resp.Broker)
	}
	if !kisBroker.called {
		t.Fatalf("expected KIS broker to be called")
	}
	if kisBroker.gotMethod != "" {
		t.Fatalf("method = %q, want empty (adapter default)", kisBroker.gotMethod)
	}
	if kisBroker.gotPath != "/uapi/overseas-price/v1/quotations/price" {
		t.Fatalf("path = %q", kisBroker.gotPath)
	}
	if kisBroker.gotTRID != "HHDFS00000300" {
		t.Fatalf("tr_id = %q", kisBroker.gotTRID)
	}
	if got := kisBroker.gotFields["EXCD"]; got != "NAS" {
		t.Fatalf("query EXCD = %q, want NAS", got)
	}
	if got := kisBroker.gotFields["SYMB"]; got != "AAPL" {
		t.Fatalf("query SYMB = %q, want AAPL", got)
	}
}

func TestHandleKISProxy_GETBodyCompatibility(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"tr_id":"FHKST03010100","body":{"FID_COND_MRKT_DIV_CODE":"J","FID_INPUT_ISCD":"KR103501GE04"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-bond/v1/quotations/inquire-price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := kisBroker.gotFields["FID_COND_MRKT_DIV_CODE"]; got != "J" {
		t.Fatalf("query FID_COND_MRKT_DIV_CODE = %q, want J", got)
	}
	if got := kisBroker.gotFields["FID_INPUT_ISCD"]; got != "KR103501GE04" {
		t.Fatalf("query FID_INPUT_ISCD = %q", got)
	}
}

func TestHandleKISProxy_StaticFlatRequestCompatibility(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"FID_COND_MRKT_DIV_CODE":"J","FID_INPUT_ISCD":"KR103501GE04"}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-bond/v1/quotations/inquire-price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := kisBroker.gotFields["FID_COND_MRKT_DIV_CODE"]; got != "J" {
		t.Fatalf("query FID_COND_MRKT_DIV_CODE = %q, want J", got)
	}
	if got := kisBroker.gotFields["FID_INPUT_ISCD"]; got != "KR103501GE04" {
		t.Fatalf("query FID_INPUT_ISCD = %q", got)
	}
}

func TestHandleKISProxy_TRIDOptional(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"params":{"BASS_DT":"20260302"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/chk-holiday", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.gotTRID != "" {
		t.Fatalf("tr_id = %q, want empty", kisBroker.gotTRID)
	}
}

func TestHandleKISProxy_InvalidAccount(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{proxyStubBroker: proxyStubBroker{name: "KIS"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"account_id":"missing","tr_id":"HHDFS00000300","params":{"EXCD":"NAS"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/overseas-price/v1/quotations/price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKISProxy_NonKISAccountRejected(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyStubBroker{name: "KIWOOM"}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"account_id":"kiwoom-acc","tr_id":"HHDFS00000300","params":{"EXCD":"NAS"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/overseas-price/v1/quotations/price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKISProxy_StrictOnlyUndocumentedPathReturnsNotFound(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"ANY":"VALUE"}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/not-documented/v1/endpoint", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.called {
		t.Fatalf("expected broker not to be called for undocumented path")
	}
}

func TestHandleKISProxy_MethodNormalizedToUpper(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"method":"get","tr_id":"HHDFS00000300","params":{"EXCD":"NAS","SYMB":"AAPL"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/overseas-price/v1/quotations/price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", kisBroker.gotMethod)
	}
}

func TestHandleKISProxy_InvalidMethodRejected(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyKISBroker{
		proxyStubBroker: proxyStubBroker{name: "KIS"},
		resp:            map[string]interface{}{"rt_cd": "0"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"method":"trace","tr_id":"HHDFS00000300","params":{"EXCD":"NAS","SYMB":"AAPL"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kis/overseas-price/v1/quotations/price", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kisBroker.called {
		t.Fatalf("expected broker not to be called")
	}
}
