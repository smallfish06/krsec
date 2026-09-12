package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	"github.com/smallfish06/krsec/pkg/ls"
)

type proxyLSBroker struct {
	proxyStubBroker
	called    bool
	gotMethod string
	gotPath   string
	gotTRCD   string
	gotReq    any
	resp      any
	err       error
}

func (b *proxyLSBroker) CallEndpoint(
	_ context.Context,
	method string,
	path string,
	trCD string,
	request any,
) (any, error) {
	b.called = true
	b.gotMethod = method
	b.gotPath = path
	b.gotTRCD = trCD
	b.gotReq = request
	return b.resp, b.err
}

func TestHandleLSProxy_DefaultRouteAndFirstLSAccount(t *testing.T) {
	t.Parallel()

	lsBroker := &proxyLSBroker{
		proxyStubBroker: proxyStubBroker{name: "LS"},
		resp:            map[string]any{"rsp_cd": "00000", "rsp_msg": "ok"},
	}
	kisBroker := &proxyStubBroker{name: "KIS"}

	s := newOrderTestServer(
		map[string]broker.Broker{
			"kis-acc": kisBroker,
			"ls-acc":  lsBroker,
		},
		[]config.AccountConfig{
			{AccountID: "kis-acc", Broker: "kis"},
			{AccountID: "ls-acc", Broker: "ls"},
		},
	)

	body := []byte(`{"tr_cd":"t1102","params":{"t1102InBlock":{"shcode":"078020","exchgubun":"K"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/market-data", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Broker != "LS" {
		t.Fatalf("broker = %q, want LS", resp.Broker)
	}
	if !lsBroker.called {
		t.Fatalf("expected LS broker to be called")
	}
	if lsBroker.gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", lsBroker.gotMethod)
	}
	if lsBroker.gotPath != "/stock/market-data" {
		t.Fatalf("path = %q", lsBroker.gotPath)
	}
	if lsBroker.gotTRCD != "t1102" {
		t.Fatalf("tr_cd = %q, want t1102", lsBroker.gotTRCD)
	}
	reqMap, ok := lsBroker.gotReq.(map[string]any)
	if !ok {
		t.Fatalf("request type = %T, want map[string]any", lsBroker.gotReq)
	}
	if _, ok := reqMap["t1102InBlock"].(map[string]any); !ok {
		t.Fatalf("request missing t1102InBlock: %#v", reqMap)
	}
}

func TestHandleLSProxy_MissingTRCD(t *testing.T) {
	t.Parallel()

	lsBroker := &proxyLSBroker{proxyStubBroker: proxyStubBroker{name: "LS"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"ls-acc": lsBroker},
		[]config.AccountConfig{{AccountID: "ls-acc", Broker: "ls"}},
	)

	body := []byte(`{"params":{"t1102InBlock":{"shcode":"078020"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/market-data", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if lsBroker.called {
		t.Fatalf("expected broker not to be called")
	}
}

func TestHandleLSProxy_NonLSAccountRejected(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyStubBroker{name: "KIS"}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"account_id":"kis-acc","tr_cd":"t1102","params":{"t1102InBlock":{"shcode":"078020"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/market-data", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleLSProxy_MethodNormalizedToUpper(t *testing.T) {
	t.Parallel()

	lsBroker := &proxyLSBroker{
		proxyStubBroker: proxyStubBroker{name: "LS"},
		resp:            map[string]any{"rsp_cd": "00000", "rsp_msg": "ok"},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"ls-acc": lsBroker},
		[]config.AccountConfig{{AccountID: "ls-acc", Broker: "ls"}},
	)

	body := []byte(`{"method":"get","tr_cd":"t1102","params":{"t1102InBlock":{"shcode":"078020"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/market-data", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if lsBroker.gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", lsBroker.gotMethod)
	}
}

func TestHandleLSProxy_InvalidMethodRejected(t *testing.T) {
	t.Parallel()

	lsBroker := &proxyLSBroker{proxyStubBroker: proxyStubBroker{name: "LS"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"ls-acc": lsBroker},
		[]config.AccountConfig{{AccountID: "ls-acc", Broker: "ls"}},
	)

	body := []byte(`{"method":"trace","tr_cd":"t1102","params":{"t1102InBlock":{"shcode":"078020"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/market-data", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if lsBroker.called {
		t.Fatalf("expected broker not to be called")
	}
}

type paginatedLSBroker struct {
	proxyLSBroker
	continuation ls.Continuation
}

func (b *paginatedLSBroker) CallEndpointPage(_ context.Context, _, _, _ string, request any, continuation ls.Continuation) (*ls.EndpointPage, error) {
	b.continuation = continuation
	b.gotReq = request
	return &ls.EndpointPage{Data: map[string]any{"rsp_cd": "00000"}, Continuation: ls.Continuation{TRCont: "Y", TRContKey: "following-page"}}, nil
}

func TestHandleLSProxyPreservesContinuationOutsidePayload(t *testing.T) {
	b := &paginatedLSBroker{proxyLSBroker: proxyLSBroker{proxyStubBroker: proxyStubBroker{name: "LS"}}}
	s := newOrderTestServer(map[string]broker.Broker{"ls": b}, []config.AccountConfig{{AccountID: "ls", Broker: "ls"}})
	req := httptest.NewRequest(http.MethodPost, "/ls/stock/accno", bytes.NewBufferString(`{"tr_cd":"t0425","tr_cont":"Y","tr_cont_key":"next-page","body":{"t0425InBlock":{"expcode":"005930"}}}`))
	rr := performFiberRequest(t, s, req)
	if rr.Code != http.StatusOK || b.continuation.TRCont != "Y" || b.continuation.TRContKey != "next-page" {
		t.Fatalf("request continuation lost: status=%d cont=%+v body=%s", rr.Code, b.continuation, rr.Body.String())
	}
	if rr.Header().Get("tr_cont") != "Y" || rr.Header().Get("tr_cont_key") != "following-page" {
		t.Fatalf("response continuation lost: %v", rr.Header())
	}
	data := decodeResponse(t, rr).Data.(map[string]any)
	if data["rsp_cd"] != "00000" || data["data"] != nil {
		t.Fatalf("existing data shape changed: %v", data)
	}
	if b.gotReq.(map[string]any)["tr_cont_key"] != nil {
		t.Fatal("header cursor leaked into upstream body")
	}
}

func TestHandleLSProxyRejectsContinuationForLegacyAdapter(t *testing.T) {
	b := &proxyLSBroker{proxyStubBroker: proxyStubBroker{name: "LS"}}
	s := newOrderTestServer(map[string]broker.Broker{"ls": b}, []config.AccountConfig{{AccountID: "ls", Broker: "ls"}})
	rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodPost, "/ls/stock/accno", bytes.NewBufferString(`{"tr_cd":"t0425","tr_cont":"Y","tr_cont_key":"next"}`)))
	if rr.Code != http.StatusBadRequest || b.called {
		t.Fatalf("legacy adapter silently ignored continuation: %d %s", rr.Code, rr.Body.String())
	}
}

func TestLSRequestContinuationHeadersAndConflicts(t *testing.T) {
	cont, err := lsRequestContinuation("Y", "cursor", "", "")
	if err != nil || cont.TRCont != "Y" || cont.TRContKey != "cursor" {
		t.Fatalf("headers ignored: %+v %v", cont, err)
	}
	for _, fields := range [][4]string{{"Y", "a", "Y", "b"}, {"Y", "a", "N", ""}, {"Y", "", "", ""}, {"", "a", "", ""}} {
		if _, err := lsRequestContinuation(fields[0], fields[1], fields[2], fields[3]); err == nil {
			t.Fatalf("invalid continuation accepted: %v", fields)
		}
	}
}
