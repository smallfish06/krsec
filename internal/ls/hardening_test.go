package ls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
)

func TestIsSuccessCode(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]bool{
		"00000":    true,
		"00040":    true, // buy order accepted
		"00039":    true, // sell order accepted
		"00136":    true, // account query complete
		"00999":    true,
		"01000":    false,
		"02705":    false,
		"08000":    false,
		"IGW00121": false,
		"Q0001":    false,
		"40":       false,
		"":         false,
	} {
		if got := isSuccessCode(code); got != want {
			t.Errorf("isSuccessCode(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestMapLSStatusSeparatesBusinessRejectionsFromFaults(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]error{
		"IGW00121": broker.ErrUnauthorized,
		"Q0001":    broker.ErrUpstreamBadRequest,
		"02705":    broker.ErrUpstreamBadRequest,
		"08001":    broker.ErrServerError,
	} {
		// A message that merely mentions permissions must not turn a business
		// rejection into an authentication failure.
		if err := mapLSStatus(code, "주문 권한이 없는 종목입니다"); !errors.Is(err, want) {
			t.Errorf("mapLSStatus(%q) = %v, want %v", code, err, want)
		}
	}
}

// lsTestClient serves one token and routes every other request to handler.
func lsTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathOAuthToken {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(ts.Close)

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}
	return c
}

// LS answers an accepted order with 00040/00039, not 00000. Reading that as a
// failure reports a live order as rejected, and the caller's retry doubles it.
func TestAcceptedOrderCodeIsNotAnError(t *testing.T) {
	c := lsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rsp_cd":              "00040",
			"rsp_msg":             "매수주문이 완료 되었습니다.",
			"CSPAT00601OutBlock2": map[string]any{"OrdNo": 12345},
		})
	})
	out, err := c.CallEndpoint(context.Background(), http.MethodPost, "/stock/order", "CSPAT00601", map[string]any{"CSPAT00601InBlock1": map[string]any{}})
	if err != nil {
		t.Fatalf("accepted order reported as failure: %v", err)
	}
	if out == nil {
		t.Fatal("accepted order returned no payload")
	}
}

// A rejection in the response body means LS processed the order. Refreshing
// the token and sending the order again could place it twice.
func TestOrderIsNeverResentAfterABodyLevelRejection(t *testing.T) {
	var orders atomic.Int32
	c := lsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		orders.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "IGW00121", "rsp_msg": "유효하지 않은 token 입니다."})
	})
	_, err := c.CallEndpoint(context.Background(), http.MethodPost, "/stock/order", "CSPAT00601", map[string]any{"CSPAT00601InBlock1": map[string]any{}})
	if !errors.Is(err, broker.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if got := orders.Load(); got != 1 {
		t.Fatalf("order sent %d times, want exactly once", got)
	}
}

// Values seen on the live API. t0424 answers tr_cont "0" on a complete
// response; rejecting it as invalid failed every balance and position call.
func TestResponseContinuationOnlyTreatsYAsMore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flag, key string
		want      Continuation
	}{
		{"Y", "0", Continuation{TRCont: "Y", TRContKey: "0"}}, // t8410, g3204
		{"y", "abc", Continuation{TRCont: "Y", TRContKey: "abc"}},
		{"N", "0", Continuation{TRCont: "N"}}, // t1102
		{"0", "", Continuation{TRCont: "N"}},  // t0424
		{"", "", Continuation{}},              // t0425
	}
	for _, tc := range cases {
		h := http.Header{}
		h.Set("tr_cont", tc.flag)
		h.Set("tr_cont_key", tc.key)
		if got := responseContinuation(h); got != tc.want {
			t.Errorf("tr_cont=%q key=%q: got %+v, want %+v", tc.flag, tc.key, got, tc.want)
		}
	}
}

func TestBalanceSurvivesNumericContinuationFlag(t *testing.T) {
	c := lsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("tr_cont", "0")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rsp_cd":         "00000",
			"rsp_msg":        "조회가 완료되었습니다.",
			"t0424OutBlock":  map[string]any{"sunamt": 1000, "cts_expcode": ""},
			"t0424OutBlock1": []any{},
		})
	})
	summary, rows, err := c.InquireBalance(context.Background())
	if err != nil {
		t.Fatalf("InquireBalance: %v", err)
	}
	if summary == nil || len(rows) != 0 {
		t.Fatalf("summary=%v rows=%v", summary, rows)
	}
}

// An account query answers 00136 and an empty history 00200; both are
// successful responses whose blocks must reach the caller.
func TestAccountQueryCodesAreSuccess(t *testing.T) {
	for _, code := range []string{"00136", "00200"} {
		c := lsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": code, "rsp_msg": "조회", "CSPAQ13700OutBlock3": []any{}})
		})
		out, err := c.CallEndpoint(context.Background(), http.MethodPost, PathStockAccount, "CSPAQ13700", map[string]any{"CSPAQ13700InBlock1": map[string]any{}})
		if err != nil {
			t.Fatalf("rsp_cd %s reported as failure: %v", code, err)
		}
		if _, ok := out["CSPAQ13700OutBlock3"]; !ok {
			t.Fatalf("rsp_cd %s: response blocks dropped", code)
		}
	}
}
