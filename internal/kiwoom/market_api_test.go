package kiwoom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

func TestCallDocumentedEndpoint_OrderBook_UsesKa10004AndMrkcondPath(t *testing.T) {
	t.Parallel()

	var gotAPIID string
	var gotSymbol string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "test-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/mrkcond":
			gotAPIID = r.Header.Get("api-id")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotSymbol = body["stk_cd"]
			_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "return_msg": "ok", "ask1": "70010"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.SetCredentials("k", "s")

	if _, err := c.CallDocumentedEndpoint(
		context.Background(),
		"ka10004",
		PathMarketCond,
		&kiwoomspecs.KiwoomApiDostkMrkcondKa10004Request{StkCd: "005930"},
	); err != nil {
		t.Fatalf("CallDocumentedEndpoint error: %v", err)
	}

	if gotAPIID != "ka10004" {
		t.Fatalf("api-id = %q, want ka10004", gotAPIID)
	}
	if gotSymbol != "005930" {
		t.Fatalf("stk_cd = %q, want 005930", gotSymbol)
	}
}

func TestCallDocumentedEndpoint_VolumeRank_UsesKa10030AndRkinfoPath(t *testing.T) {
	t.Parallel()

	var gotAPIID string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "test-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/rkinfo":
			gotAPIID = r.Header.Get("api-id")
			_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "return_msg": "ok", "output": []map[string]any{{"stk_cd": "005930"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.SetCredentials("k", "s")

	if _, err := c.CallDocumentedEndpoint(context.Background(), "ka10030", PathRankingInfo, &kiwoomspecs.KiwoomApiDostkRkinfoKa10030Request{
		StexTp:       "0",
		MrktTp:       "000",
		SortTp:       "1",
		MangStkIncls: "0",
		CrdTp:        "0",
		TrdeQtyTp:    "0",
		PricTp:       "0",
		TrdePricaTp:  "0",
		MrktOpenTp:   "0",
	}); err != nil {
		t.Fatalf("CallDocumentedEndpoint error: %v", err)
	}

	if gotAPIID != "ka10030" {
		t.Fatalf("api-id = %q, want ka10030", gotAPIID)
	}
}

func TestCallDocumentedEndpoint_SectorCurrentAndDepositDetail(t *testing.T) {
	t.Parallel()

	var sectorAPIID string
	var accountAPIID string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "test-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/sect":
			sectorAPIID = r.Header.Get("api-id")
			_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "return_msg": "ok"})
		case "/api/dostk/acnt":
			accountAPIID = r.Header.Get("api-id")
			_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "return_msg": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.SetCredentials("k", "s")

	if _, err := c.CallDocumentedEndpoint(
		context.Background(),
		"ka20001",
		PathSector,
		&kiwoomspecs.KiwoomApiDostkSectKa20001Request{IndsCd: "001", MrktTp: "000"},
	); err != nil {
		t.Fatalf("CallDocumentedEndpoint error: %v", err)
	}
	if _, err := c.CallDocumentedEndpoint(
		context.Background(),
		"kt00001",
		PathAccount,
		&kiwoomspecs.KiwoomApiDostkAcntKt00001Request{QryTp: "0"},
	); err != nil {
		t.Fatalf("CallDocumentedEndpoint error: %v", err)
	}

	if sectorAPIID != "ka20001" {
		t.Fatalf("sector api-id = %q, want ka20001", sectorAPIID)
	}
	if accountAPIID != "kt00001" {
		t.Fatalf("account api-id = %q, want kt00001", accountAPIID)
	}
}

func TestInquireOrderExecutionsByExchange_UsesExchangeBody(t *testing.T) {
	t.Parallel()

	var gotAPIID string
	var gotExchange string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "test-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/acnt":
			gotAPIID = r.Header.Get("api-id")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotExchange = body["stex_tp"]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"return_code": 0,
				"return_msg":  "ok",
				"cntr": []map[string]any{
					{"ord_no": "1", "stk_cd": "005930", "cntr_qty": "1", "cntr_pric": "70000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.SetCredentials("k", "s")

	if _, err := c.InquireOrderExecutionsByExchange(context.Background(), "005930", "2"); err != nil {
		t.Fatalf("InquireOrderExecutionsByExchange error: %v", err)
	}
	if gotAPIID != "ka10076" {
		t.Fatalf("api-id = %q, want ka10076", gotAPIID)
	}
	if gotExchange != "2" {
		t.Fatalf("stex_tp = %q, want 2", gotExchange)
	}
}
