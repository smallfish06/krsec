package ls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func paginationTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.setToken("local-test-token", time.Now().Add(time.Hour))
	c.limiter = nil
	return c
}

func TestCallEndpointPagePreservesContinuation(t *testing.T) {
	calls := 0
	c := paginationTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			if r.Header.Get("tr_cont") != "N" || r.Header.Get("tr_cont_key") != "" {
				t.Error("unexpected first-page headers")
			}
			w.Header().Set("tr_cont", "Y")
			w.Header().Set("tr_cont_key", "page-two")
		} else {
			if r.Header.Get("tr_cont") != "Y" || r.Header.Get("tr_cont_key") != "page-two" {
				t.Error("next-page headers were lost")
			}
			w.Header().Set("tr_cont", "N")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "page": calls})
	})
	first, err := c.CallEndpointPage(context.Background(), "POST", PathStockAccount, "t0425", map[string]any{}, Continuation{})
	if err != nil {
		t.Fatal(err)
	}
	if first.TRCont != "Y" || first.TRContKey != "page-two" {
		t.Fatalf("missing response cursor: %+v", first)
	}
	second, err := c.CallEndpointPage(context.Background(), "POST", PathStockAccount, "t0425", map[string]any{}, first.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	if second.Data["page"] != json.Number("2") || second.TRCont != "N" || calls != 2 {
		t.Fatalf("wrong second page: %+v", second)
	}
}

func TestCallEndpointPreservesExactNumericIdentifiers(t *testing.T) {
	c := paginationTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rsp_cd":"00000","order":{"OrdNo":1234567890,"quantity":9007199254740993}}`))
	})
	data, err := c.CallEndpoint(context.Background(), "POST", PathStockAccount, "t0425", nil)
	if err != nil {
		t.Fatal(err)
	}
	order := data["order"].(map[string]any)
	if order["OrdNo"] != json.Number("1234567890") || order["quantity"] != json.Number("9007199254740993") {
		t.Fatalf("numeric identifier lost precision: %#v", order)
	}
}

func TestCallEndpointPageRejectsInvalidContinuationBeforeRequest(t *testing.T) {
	c := paginationTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid continuation sent upstream") })
	for _, cont := range []Continuation{{TRCont: "Y"}, {TRCont: "invalid"}, {TRContKey: "cursor"}} {
		if _, err := c.CallEndpointPage(context.Background(), "POST", PathStockAccount, "t0425", nil, cont); err == nil {
			t.Fatalf("accepted invalid cursor: %+v", cont)
		}
	}
}

func TestInquireBalanceTraversesBodyAndHeaderContinuation(t *testing.T) {
	for _, headerCursor := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "headers"}[headerCursor], func(t *testing.T) {
			calls := 0
			c := paginationTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				cursor := ""
				if calls == 1 {
					cursor = "next-symbol"
				}
				if calls == 2 && !headerCursor && body["t0424InBlock"]["cts_expcode"] != "next-symbol" {
					t.Error("body cursor missing")
				}
				if headerCursor {
					cursor = ""
					if calls == 1 {
						w.Header().Set("tr_cont", "Y")
						w.Header().Set("tr_cont_key", "next-header")
					} else if r.Header.Get("tr_cont_key") != "next-header" {
						t.Error("header cursor missing")
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "t0424OutBlock": map[string]any{"cts_expcode": cursor, "sunamt": 100}, "t0424OutBlock1": []any{map[string]any{"expcode": calls, "janqty": 1}}})
			})
			_, rows, err := c.InquireBalance(context.Background())
			if err != nil || len(rows) != 2 || calls != 2 {
				t.Fatalf("incomplete holdings: rows=%v calls=%d err=%v", rows, calls, err)
			}
		})
	}
}

func TestInquireBalanceDoesNotReturnPartialRows(t *testing.T) {
	for _, scenario := range []string{"second page fails", "repeated cursor", "missing cursor", "invalid flag", "invalid row"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := paginationTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if scenario == "second page fails" && calls == 2 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"rsp_cd":"Q001","rsp_msg":"invalid page"}`))
					return
				}
				if scenario == "invalid flag" {
					w.Header().Set("tr_cont", "X")
				}
				if scenario == "invalid row" {
					_, _ = w.Write([]byte(`{"rsp_cd":"00000","t0424OutBlock":{},"t0424OutBlock1":[{"expcode":"005930"},null]}`))
					return
				}
				cursor := "repeat"
				if scenario == "missing cursor" {
					cursor = ""
					w.Header().Set("tr_cont", "Y")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "t0424OutBlock": map[string]any{"cts_expcode": cursor}, "t0424OutBlock1": []any{map[string]any{"expcode": "005930"}}})
			})
			summary, rows, err := c.InquireBalance(context.Background())
			if err == nil || summary != nil || rows != nil || calls > 2 {
				t.Fatalf("partial result escaped: summary=%v rows=%v calls=%d err=%v", summary, rows, calls, err)
			}
		})
	}
}
