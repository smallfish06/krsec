package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	internalls "github.com/smallfish06/krsec/internal/ls"
	"github.com/smallfish06/krsec/pkg/broker"
)

func TestAdapterCallEndpoint_DispatchesDocumentedRESTTR(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotTRCD string
	var gotExchange string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathStockMarket:
			gotMethod = r.Method
			gotTRCD = r.Header.Get("tr_cd")
			var body map[string]map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode body: %v", err)
			}
			gotExchange = body["t1102InBlock"]["exchgubun"]
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"t1102OutBlock": map[string]interface{}{
					"shcode": "078020",
					"price":  "1234",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	resp, err := a.CallEndpoint(context.Background(), "", internalls.PathStockMarket, internalls.TRStockQuote, map[string]interface{}{
		"t1102InBlock": map[string]interface{}{
			"shcode":    "078020",
			"exchgubun": "K",
		},
	})
	if err != nil {
		t.Fatalf("CallEndpoint error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotTRCD != internalls.TRStockQuote {
		t.Fatalf("tr_cd = %q, want %s", gotTRCD, internalls.TRStockQuote)
	}
	if gotExchange != "K" {
		t.Fatalf("exchgubun = %q, want K", gotExchange)
	}
	out, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response type = %T, want map[string]interface{}", resp)
	}
	if _, ok := out["t1102OutBlock"].(map[string]interface{}); !ok {
		t.Fatalf("missing t1102OutBlock: %#v", out)
	}
}

func TestAdapterCallEndpoint_ValidatesDocumentedRequiredFields(t *testing.T) {
	t.Parallel()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	_, err := a.CallEndpoint(context.Background(), http.MethodPost, internalls.PathStockMarket, internalls.TRStockQuote, map[string]interface{}{
		"t1102InBlock": map[string]interface{}{
			"shcode": "078020",
		},
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("error = %v, want ErrInvalidOrderRequest", err)
	}
	if !strings.Contains(err.Error(), "exchgubun") {
		t.Fatalf("error = %v, want missing exchgubun", err)
	}
}

func TestAdapterCallEndpoint_RejectsUndocumentedTR(t *testing.T) {
	t.Parallel()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	_, err := a.CallEndpoint(context.Background(), http.MethodPost, internalls.PathStockMarket, "NOPE", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected unsupported endpoint error, got nil")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("error = %v, want ErrInvalidOrderRequest", err)
	}
}

func TestAdapterCallEndpoint_RejectsWebSocketTRForRESTDispatch(t *testing.T) {
	t.Parallel()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	_, err := a.CallEndpoint(context.Background(), http.MethodPost, "/websocket/overseas-stock", internalls.TRRealtimeOverseasTrade, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected websocket rejection, got nil")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("error = %v, want ErrInvalidOrderRequest", err)
	}
	if !strings.Contains(err.Error(), "ConnectRealtime") {
		t.Fatalf("error = %v, want ConnectRealtime hint", err)
	}
}
