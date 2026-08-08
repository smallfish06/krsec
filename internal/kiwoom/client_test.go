package kiwoom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

type memoryTokenManager struct {
	token     string
	expiresAt time.Time
	hasToken  bool

	setCalls  int
	waitCalls int
}

func (m *memoryTokenManager) GetToken(string) (string, time.Time, bool) {
	if m.hasToken {
		return m.token, m.expiresAt, true
	}
	return "", time.Time{}, false
}

func (m *memoryTokenManager) SetToken(_ string, token string, expiresAt time.Time) error {
	m.token = token
	m.expiresAt = expiresAt
	m.hasToken = true
	m.setCalls++
	return nil
}

func (m *memoryTokenManager) DeleteToken(string) error {
	m.token = ""
	m.expiresAt = time.Time{}
	m.hasToken = false
	return nil
}

func (m *memoryTokenManager) WaitForAuth(string) {
	m.waitCalls++
}

func TestClientInquirePrice_UsesAuthAndAPIIDHeader(t *testing.T) {
	var gotAuth string
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
		case "/api/dostk/stkinfo":
			gotAuth = r.Header.Get("Authorization")
			gotAPIID = r.Header.Get("api-id")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotSymbol = body["stk_cd"]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stk_cd":      "005930",
				"cur_prc":     "70000",
				"open_pric":   "69500",
				"high_pric":   "70500",
				"low_pric":    "69000",
				"pred_pre":    "500",
				"flu_rt":      "0.72",
				"trde_qty":    "12345",
				"base_pric":   "69500",
				"upl_pric":    "91000",
				"lst_pric":    "48000",
				"return_code": 0,
				"return_msg":  "ok",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tm := &memoryTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.SetBaseURL(ts.URL)

	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "k", AppSecret: "s"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	quote, err := c.InquirePrice(context.Background(), "005930")
	if err != nil {
		t.Fatalf("InquirePrice error: %v", err)
	}

	if tm.setCalls != 1 {
		t.Fatalf("SetToken calls = %d, want 1", tm.setCalls)
	}
	if !strings.HasPrefix(gotAuth, "Bearer test-token") {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotAPIID != "ka10001" {
		t.Fatalf("api-id header = %q, want ka10001", gotAPIID)
	}
	if gotSymbol != "005930" {
		t.Fatalf("stk_cd = %q, want 005930", gotSymbol)
	}
	if asFloat64(quote.CurPrc) != 70000 || asFloat64(quote.PredPre) != 500 || asFloat64(quote.FluRt) != 0.72 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}

func TestClientInquirePrice_ReturnCodeError(t *testing.T) {
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
		case "/api/dostk/stkinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"return_code": -301,
				"return_msg":  "invalid symbol",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	c.SetCredentials("k", "s")

	_, err := c.InquirePrice(context.Background(), "BAD")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid symbol") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, broker.ErrInvalidSymbol) {
		t.Fatalf("expected ErrInvalidSymbol mapping, got: %v", err)
	}
}

func TestClientPlaceStockOrder_SellUsesSellTR(t *testing.T) {
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
		case "/api/dostk/ordr":
			gotAPIID = r.Header.Get("api-id")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ord_no":      "0001234",
				"return_code": 0,
				"return_msg":  "ok",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "k", AppSecret: "s"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := c.PlaceStockOrder(context.Background(), StockOrderSideSell, kiwoomspecs.KiwoomApiDostkOrdrKt10000Request{
		DmstStexTp: "KRX",
		StkCd:      "005930",
		OrdQty:     "1",
		OrdUv:      "70000",
		TrdeTp:     "0",
	})
	if err != nil {
		t.Fatalf("PlaceStockOrder error: %v", err)
	}
	if gotAPIID != "kt10001" {
		t.Fatalf("api-id header = %q, want kt10001", gotAPIID)
	}
}

func TestAuthenticate_InvalidCredentialsMapped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"return_code": 401,
			"return_msg":  "appkey invalid",
		})
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)

	_, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "bad", AppSecret: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, broker.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials mapping, got: %v", err)
	}
}

func TestCallDocumentedEndpoint_UsesGeneratedMethod(t *testing.T) {
	var gotMethod string
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
		case "/api/dostk/stkinfo":
			gotMethod = r.Method
			gotAPIID = r.Header.Get("api-id")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotSymbol = asString(body["stk_cd"])
			_ = json.NewEncoder(w).Encode(map[string]any{
				"return_code": 0,
				"return_msg":  "ok",
				"name":        "sample",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "k", AppSecret: "s"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := c.CallDocumentedEndpoint(
		context.Background(),
		"ka10002",
		"/api/dostk/stkinfo",
		&kiwoomspecs.KiwoomApiDostkStkinfoKa10002Request{StkCd: "005930"},
	)
	if err != nil {
		t.Fatalf("CallDocumentedEndpoint error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotAPIID != "ka10002" {
		t.Fatalf("api-id header = %q, want ka10002", gotAPIID)
	}
	if gotSymbol != "005930" {
		t.Fatalf("stk_cd = %q, want 005930", gotSymbol)
	}
}

func TestCallDocumentedEndpoint_MissingSpec(t *testing.T) {
	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetCredentials("k", "s")

	_, err := c.CallDocumentedEndpoint(
		context.Background(),
		"zz99999",
		"/api/dostk/stkinfo",
		&kiwoomspecs.KiwoomApiDostkStkinfoKa10001Request{StkCd: "005930"},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing documented endpoint spec") {
		t.Fatalf("unexpected error: %v", err)
	}
}
