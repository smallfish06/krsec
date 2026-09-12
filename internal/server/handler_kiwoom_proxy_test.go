package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

type proxyKiwoomBroker struct {
	proxyStubBroker
	called    bool
	gotMethod string
	gotPath   string
	gotAPIID  string
	gotReq    any
	resp      any
	err       error
}

func (b *proxyKiwoomBroker) CallEndpoint(
	_ context.Context,
	method string,
	path string,
	apiID string,
	request any,
) (any, error) {
	b.called = true
	b.gotMethod = method
	b.gotPath = path
	b.gotAPIID = apiID
	b.gotReq = request
	return b.resp, b.err
}

func TestHandleKiwoomProxy_DefaultRouteAndFirstKiwoomAccount(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{
		proxyStubBroker: proxyStubBroker{name: "KIWOOM"},
		resp:            map[string]any{"return_code": 0, "return_msg": "ok"},
	}
	kisBroker := &proxyStubBroker{name: "KIS"}

	s := newOrderTestServer(
		map[string]broker.Broker{
			"kis-acc":    kisBroker,
			"kiwoom-acc": kiwoomBroker,
		},
		[]config.AccountConfig{
			{AccountID: "kis-acc", Broker: "kis"},
			{AccountID: "kiwoom-acc", Broker: "kiwoom"},
		},
	)

	body := []byte(`{"api_id":"ka10001","params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Broker != "KIWOOM" {
		t.Fatalf("broker = %q, want KIWOOM", resp.Broker)
	}
	if !kiwoomBroker.called {
		t.Fatalf("expected Kiwoom broker to be called")
	}
	if kiwoomBroker.gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", kiwoomBroker.gotMethod)
	}
	if kiwoomBroker.gotPath != "/api/dostk/stkinfo" {
		t.Fatalf("path = %q", kiwoomBroker.gotPath)
	}
	if kiwoomBroker.gotAPIID != "ka10001" {
		t.Fatalf("api_id = %q, want ka10001", kiwoomBroker.gotAPIID)
	}
	reqMap, ok := kiwoomBroker.gotReq.(map[string]any)
	if !ok {
		t.Fatalf("request type = %T, want map[string]any", kiwoomBroker.gotReq)
	}
	if got, ok := reqMap["stk_cd"].(string); !ok || got != "005930" {
		t.Fatalf("params stk_cd = %#v, want 005930", reqMap["stk_cd"])
	}
}

func TestHandleKiwoomProxy_MissingAPIID(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKiwoomProxy_InvalidAccount(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"account_id":"missing","api_id":"ka10001","params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKiwoomProxy_NonKiwoomAccountRejected(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyStubBroker{name: "KIS"}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"account_id":"kis-acc","api_id":"ka10001","params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKiwoomProxy_StaticRoute(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{
		proxyStubBroker: proxyStubBroker{name: "KIWOOM"},
		resp:            map[string]any{"return_code": 0, "return_msg": "ok"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"stk_cd":"005930"}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo/ka10001?account_id=kiwoom-acc", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !kiwoomBroker.called {
		t.Fatalf("expected Kiwoom broker to be called")
	}
	if kiwoomBroker.gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", kiwoomBroker.gotMethod)
	}
	if kiwoomBroker.gotPath != "/api/dostk/stkinfo" {
		t.Fatalf("path = %q", kiwoomBroker.gotPath)
	}
	if kiwoomBroker.gotAPIID != "ka10001" {
		t.Fatalf("api_id = %q, want ka10001", kiwoomBroker.gotAPIID)
	}
	reqMap, ok := kiwoomBroker.gotReq.(map[string]any)
	if !ok {
		t.Fatalf("request type = %T, want map[string]any", kiwoomBroker.gotReq)
	}
	if got, ok := reqMap["stk_cd"].(string); !ok || got != "005930" {
		t.Fatalf("request stk_cd = %#v, want 005930", reqMap["stk_cd"])
	}
}

func TestHandleKiwoomProxy_MethodNormalizedToUpper(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{
		proxyStubBroker: proxyStubBroker{name: "KIWOOM"},
		resp:            map[string]any{"return_code": 0, "return_msg": "ok"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"method":"get","api_id":"ka10001","params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kiwoomBroker.gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", kiwoomBroker.gotMethod)
	}
}

func TestHandleKiwoomProxy_InvalidMethodRejected(t *testing.T) {
	t.Parallel()

	kiwoomBroker := &proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"kiwoom-acc": kiwoomBroker},
		[]config.AccountConfig{{AccountID: "kiwoom-acc", Broker: "kiwoom"}},
	)

	body := []byte(`{"method":"trace","api_id":"ka10001","params":{"stk_cd":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if kiwoomBroker.called {
		t.Fatalf("expected broker not to be called")
	}
}

type proxyPagedKiwoomBroker struct {
	proxyKiwoomBroker
	continuation kiwoomspecs.Continuation
	next         kiwoomspecs.Continuation
}

func (b *proxyPagedKiwoomBroker) CallEndpointPage(ctx context.Context, method, path, apiID string, request any, continuation kiwoomspecs.Continuation) (*kiwoomspecs.EndpointPage, error) {
	b.continuation = continuation
	data, err := b.CallEndpoint(ctx, method, path, apiID, request)
	if err != nil {
		return nil, err
	}
	return &kiwoomspecs.EndpointPage{Data: data, Continuation: b.next}, nil
}

func TestHandleKiwoomProxyContinuationHeaders(t *testing.T) {
	for _, route := range []struct{ path, body string }{
		{"/kiwoom/dostk/chart", `{"api_id":"ka10081","body":{"stk_cd":"005930","base_dt":"20260912","upd_stkpc_tp":"1"}}`},
		{"/kiwoom/dostk/chart/ka10081", `{"stk_cd":"005930","base_dt":"20260912","upd_stkpc_tp":"1"}`},
	} {
		t.Run(route.path, func(t *testing.T) {
			brk := &proxyPagedKiwoomBroker{proxyKiwoomBroker: proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}, resp: map[string]any{"stk_cd": "005930"}}, next: kiwoomspecs.Continuation{ContYN: "Y", NextKey: "page2"}}
			s := newOrderTestServer(map[string]broker.Broker{"kiwoom": brk}, []config.AccountConfig{{AccountID: "kiwoom", Broker: "kiwoom"}})
			req := httptest.NewRequest(http.MethodPost, route.path, bytes.NewBufferString(route.body))
			req.Header.Set("cont-yn", "Y")
			req.Header.Set("next-key", "page1")
			rr := performFiberRequest(t, s, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if brk.continuation.ContYN != "Y" || brk.continuation.NextKey != "page1" {
				t.Fatalf("continuation=%+v", brk.continuation)
			}
			if rr.Header().Get("cont-yn") != "Y" || rr.Header().Get("next-key") != "page2" {
				t.Fatalf("headers=%v", rr.Header())
			}
			response := decodeResponse(t, rr)
			data, ok := response.Data.(map[string]any)
			if !ok || data["stk_cd"] != "005930" || data["data"] != nil {
				t.Fatalf("legacy body changed: %+v", response)
			}
		})
	}
}

func TestHandleKiwoomProxyContinuationBody(t *testing.T) {
	brk := &proxyPagedKiwoomBroker{proxyKiwoomBroker: proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}, resp: map[string]any{}}, next: kiwoomspecs.Continuation{ContYN: "N"}}
	s := newOrderTestServer(map[string]broker.Broker{"kiwoom": brk}, []config.AccountConfig{{AccountID: "kiwoom", Broker: "kiwoom"}})
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/chart", bytes.NewBufferString(`{"api_id":"ka10081","body":{"stk_cd":"005930"},"continuation":{"cont_yn":"Y","next_key":"page1"}}`))
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK || brk.continuation.NextKey != "page1" || rr.Header().Get("cont-yn") != "N" {
		t.Fatalf("response=%s continuation=%+v headers=%v", rr.Body.String(), brk.continuation, rr.Header())
	}
}

func TestHandleKiwoomProxyRejectsBadContinuation(t *testing.T) {
	for _, test := range []struct{ cont, key, body string }{
		{"Y", "", `{"api_id":"ka10001","body":{"stk_cd":"005930"}}`},
		{"N", "page1", `{"api_id":"ka10001","body":{"stk_cd":"005930"}}`},
		{"Y", "page1", `{"api_id":"ka10001","body":{"stk_cd":"005930"},"continuation":{"cont_yn":"Y","next_key":"other"}}`},
	} {
		brk := &proxyPagedKiwoomBroker{proxyKiwoomBroker: proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}}}
		s := newOrderTestServer(map[string]broker.Broker{"kiwoom": brk}, []config.AccountConfig{{AccountID: "kiwoom", Broker: "kiwoom"}})
		req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo", bytes.NewBufferString(test.body))
		req.Header.Set("cont-yn", test.cont)
		req.Header.Set("next-key", test.key)
		rr := performFiberRequest(t, s, req)
		if rr.Code != http.StatusBadRequest || brk.called {
			t.Fatalf("status=%d called=%v body=%s", rr.Code, brk.called, rr.Body.String())
		}
	}
}

func TestHandleKiwoomLegacyAdapterRejectsCursor(t *testing.T) {
	brk := &proxyKiwoomBroker{proxyStubBroker: proxyStubBroker{name: "KIWOOM"}}
	s := newOrderTestServer(map[string]broker.Broker{"kiwoom": brk}, []config.AccountConfig{{AccountID: "kiwoom", Broker: "kiwoom"}})
	req := httptest.NewRequest(http.MethodPost, "/kiwoom/dostk/stkinfo/ka10001", bytes.NewBufferString(`{"stk_cd":"005930"}`))
	req.Header.Set("cont-yn", "Y")
	req.Header.Set("next-key", "page1")
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusBadRequest || brk.called {
		t.Fatalf("status=%d called=%v body=%s", rr.Code, brk.called, rr.Body.String())
	}
}
