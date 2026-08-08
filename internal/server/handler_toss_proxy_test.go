package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

type proxyTossBroker struct {
	proxyStubBroker
	called    bool
	gotMethod string
	gotPath   string
	gotQuery  map[string]any
	gotBody   any
	resp      any
	err       error
}

func (b *proxyTossBroker) CallEndpoint(
	_ context.Context,
	method string,
	path string,
	query map[string]any,
	body any,
) (any, error) {
	b.called = true
	b.gotMethod = method
	b.gotPath = path
	b.gotQuery = query
	b.gotBody = body
	return b.resp, b.err
}

func TestHandleTossProxy_DefaultMethodAndFirstTossAccount(t *testing.T) {
	t.Parallel()

	tossBroker := &proxyTossBroker{
		proxyStubBroker: proxyStubBroker{name: "TOSS"},
		resp:            map[string]any{"result": []any{}},
	}
	kisBroker := &proxyStubBroker{name: "KIS"}
	s := newOrderTestServer(
		map[string]broker.Broker{
			"kis-acc":  kisBroker,
			"toss-acc": tossBroker,
		},
		[]config.AccountConfig{
			{AccountID: "kis-acc", Broker: "kis"},
			{AccountID: "toss-acc", Broker: "toss", AccountSeq: "1"},
		},
	)

	body := []byte(`{"query":{"symbols":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/toss/api/v1/prices", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tossBroker.called {
		t.Fatalf("expected Toss broker to be called")
	}
	if tossBroker.gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", tossBroker.gotMethod)
	}
	if tossBroker.gotPath != "/api/v1/prices" {
		t.Fatalf("path = %q", tossBroker.gotPath)
	}
	if tossBroker.gotQuery["symbols"] != "005930" {
		t.Fatalf("query = %#v", tossBroker.gotQuery)
	}
}

func TestHandleTossProxy_AmbiguousPathRequiresMethod(t *testing.T) {
	t.Parallel()

	tossBroker := &proxyTossBroker{proxyStubBroker: proxyStubBroker{name: "TOSS"}}
	s := newOrderTestServer(
		map[string]broker.Broker{"toss-acc": tossBroker},
		[]config.AccountConfig{{AccountID: "toss-acc", Broker: "toss", AccountSeq: "1"}},
	)

	req := httptest.NewRequest(http.MethodPost, "/toss/api/v1/orders", bytes.NewReader([]byte(`{}`)))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if tossBroker.called {
		t.Fatalf("expected broker not to be called")
	}
}

func TestHandleTossProxy_TemplatePathAndAccountScopedPost(t *testing.T) {
	t.Parallel()

	tossBroker := &proxyTossBroker{
		proxyStubBroker: proxyStubBroker{name: "TOSS"},
		resp:            map[string]any{"result": map[string]any{"orderId": "new-id"}},
	}
	s := newOrderTestServer(
		map[string]broker.Broker{"toss-acc": tossBroker},
		[]config.AccountConfig{{AccountID: "toss-acc", Broker: "toss", AccountSeq: "1"}},
	)

	body := []byte(`{"account_id":"toss-acc","method":"post","body":{"orderType":"LIMIT","price":"71000"}}`)
	req := httptest.NewRequest(http.MethodPost, "/toss/api/v1/orders/order-1/modify", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if tossBroker.gotMethod != http.MethodPost {
		t.Fatalf("method = %q", tossBroker.gotMethod)
	}
	if tossBroker.gotPath != "/api/v1/orders/order-1/modify" {
		t.Fatalf("path = %q", tossBroker.gotPath)
	}
	bodyMap, ok := tossBroker.gotBody.(map[string]any)
	if !ok || bodyMap["price"] != "71000" {
		t.Fatalf("body = %#v", tossBroker.gotBody)
	}
}

func TestHandleTossProxy_NonTossAccountRejected(t *testing.T) {
	t.Parallel()

	kisBroker := &proxyStubBroker{name: "KIS"}
	s := newOrderTestServer(
		map[string]broker.Broker{"kis-acc": kisBroker},
		[]config.AccountConfig{{AccountID: "kis-acc", Broker: "kis"}},
	)

	body := []byte(`{"account_id":"kis-acc","query":{"symbols":"005930"}}`)
	req := httptest.NewRequest(http.MethodPost, "/toss/api/v1/prices", bytes.NewReader(body))
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
