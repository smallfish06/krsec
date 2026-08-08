package ls

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
)

type memoryTokenManager struct {
	token     string
	expiresAt time.Time
	hasToken  bool

	setCalls  int
	waitCalls int
	delCalls  int
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
	m.delCalls++
	return nil
}

func (m *memoryTokenManager) WaitForAuth(string) {
	m.waitCalls++
}

func TestClientCallEndpoint_AuthenticatesAndSendsTRHeaders(t *testing.T) {
	var gotFormAppKey string
	var gotFormSecret string
	var gotAuth string
	var gotTRCD string
	var gotTRCont string
	var gotMAC string
	var gotSymbol string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			gotFormAppKey = r.Form.Get("appkey")
			gotFormSecret = r.Form.Get("appsecretkey")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"scope":        "oob",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case PathStockMarket:
			gotAuth = r.Header.Get("authorization")
			gotTRCD = r.Header.Get("tr_cd")
			gotTRCont = r.Header.Get("tr_cont")
			gotMAC = r.Header.Get("mac_address")

			var body map[string]map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode body: %v", err)
			}
			gotSymbol = body["t1102InBlock"]["shcode"]

			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"t1102OutBlock": map[string]any{
					"shcode": "078020",
					"price":  "1234",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tm := &memoryTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.SetBaseURL(ts.URL)
	c.SetMACAddress("00:11")

	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	resp, err := c.CallEndpoint(context.Background(), http.MethodPost, PathStockMarket, TRStockQuote, map[string]any{
		"t1102InBlock": map[string]string{
			"shcode":    "078020",
			"exchgubun": "K",
		},
	})
	if err != nil {
		t.Fatalf("CallEndpoint error: %v", err)
	}

	if tm.waitCalls != 1 || tm.setCalls != 1 {
		t.Fatalf("token manager calls wait=%d set=%d, want 1/1", tm.waitCalls, tm.setCalls)
	}
	if gotFormAppKey != "app-key" || gotFormSecret != "app-secret" {
		t.Fatalf("auth form appkey=%q secret=%q", gotFormAppKey, gotFormSecret)
	}
	if !strings.HasPrefix(gotAuth, "Bearer test-token") {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotTRCD != TRStockQuote {
		t.Fatalf("tr_cd = %q, want %s", gotTRCD, TRStockQuote)
	}
	if gotTRCont != "N" {
		t.Fatalf("tr_cont = %q, want N", gotTRCont)
	}
	if gotMAC != "00:11" {
		t.Fatalf("mac_address = %q", gotMAC)
	}
	if gotSymbol != "078020" {
		t.Fatalf("symbol = %q, want 078020", gotSymbol)
	}
	if _, ok := resp["t1102OutBlock"].(map[string]any); !ok {
		t.Fatalf("missing output block: %#v", resp)
	}
}

func TestClientCallEndpoint_MapsLSStatusError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case PathStockMarket:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "Q0001",
				"rsp_msg": "bad request",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := c.CallEndpoint(context.Background(), http.MethodPost, PathStockMarket, TRStockQuote, nil)
	if err == nil {
		t.Fatal("CallEndpoint expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upstream bad request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientCallEndpoint_RejectsEmptyStatusOnlyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case PathOverseasStockMarket:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "",
				"rsp_msg": "",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := NewClientWithTokenManager(false, &memoryTokenManager{})
	c.SetBaseURL(ts.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := c.CallEndpoint(context.Background(), http.MethodPost, PathOverseasStockMarket, TROverseasStockQuote, map[string]any{
		"g3101InBlock": map[string]any{
			"delaygb":   "R",
			"keysymbol": "82AAPL",
			"exchcd":    "82",
			"symbol":    "AAPL",
		},
	})
	if !errors.Is(err, broker.ErrServerError) {
		t.Fatalf("error = %v, want ErrServerError", err)
	}
	if !strings.Contains(err.Error(), "LS empty response for tr_cd g3101") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientCallEndpoint_RetriesAfterInvalidCachedToken(t *testing.T) {
	var authCalls int
	var quoteCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			authCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fresh-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case PathStockMarket:
			quoteCalls++
			if r.Header.Get("authorization") == "Bearer stale-token" {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"rsp_msg": "유효하지 않은 token 입니다.",
				})
				return
			}
			if r.Header.Get("authorization") != "Bearer fresh-token" {
				t.Fatalf("authorization = %q", r.Header.Get("authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tm := &memoryTokenManager{
		token:     "stale-token",
		expiresAt: time.Now().Add(time.Hour),
		hasToken:  true,
	}
	c := NewClientWithTokenManager(false, tm)
	c.SetBaseURL(ts.URL)
	c.SetCredentials("app-key", "app-secret")

	if _, err := c.CallEndpoint(context.Background(), http.MethodPost, PathStockMarket, TRStockQuote, nil); err != nil {
		t.Fatalf("CallEndpoint error: %v", err)
	}
	if quoteCalls != 2 {
		t.Fatalf("quote calls = %d, want 2", quoteCalls)
	}
	if authCalls != 1 {
		t.Fatalf("auth calls = %d, want 1", authCalls)
	}
	if tm.delCalls != 1 {
		t.Fatalf("DeleteToken calls = %d, want 1", tm.delCalls)
	}
}

func TestLSTRRateLimit_UsesDocumentedChartQuotas(t *testing.T) {
	t.Parallel()

	rps, burst, ok := lsTRRateLimit(TRStockChart)
	if !ok || rps != 1 || burst != 1 {
		t.Fatalf("t8410 limit = rps %v burst %d ok %v, want 1/1/true", rps, burst, ok)
	}

	rps, burst, ok = lsTRRateLimit(TROverseasStockChart)
	if !ok || rps != 1 || burst != 1 {
		t.Fatalf("g3204 limit = rps %v burst %d ok %v, want 1/1/true", rps, burst, ok)
	}

	rps, burst, ok = lsTRRateLimit(TRForeignIndexQuote)
	if !ok || rps != 1 || burst != 1 {
		t.Fatalf("t3521 limit = rps %v burst %d ok %v, want 1/1/true", rps, burst, ok)
	}

	rps, burst, ok = lsTRRateLimit(TRForeignIndexHistory)
	if !ok || rps != 1 || burst != 1 {
		t.Fatalf("t3518 limit = rps %v burst %d ok %v, want 1/1/true", rps, burst, ok)
	}

	rps, burst, ok = lsTRRateLimit(TROverseasStockQuote)
	if !ok || rps != 10 || burst != 1 {
		t.Fatalf("g3101 limit = rps %v burst %d ok %v, want 10/1/true", rps, burst, ok)
	}

	if _, _, ok := lsTRRateLimit(TRStockQuote); ok {
		t.Fatalf("domestic stock quote should use the shared client limiter only")
	}
}

func TestOverseasRealtimeKey_PadsToDocumentedWidth(t *testing.T) {
	key, err := OverseasRealtimeKey("82", "AAPL")
	if err != nil {
		t.Fatalf("OverseasRealtimeKey error: %v", err)
	}
	if len(key) != 18 {
		t.Fatalf("key length = %d, want 18", len(key))
	}
	if !strings.HasPrefix(key, "82AAPL") {
		t.Fatalf("key = %q, want 82AAPL prefix", key)
	}
	if !strings.HasSuffix(key, "            ") {
		t.Fatalf("key = %q, want trailing spaces", key)
	}
}
