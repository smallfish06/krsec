package toss

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

type memoryTokenManager struct {
	mu     sync.RWMutex
	tokens map[string]struct {
		token     string
		expiresAt time.Time
	}
}

func newMemoryTokenManager() *memoryTokenManager {
	return &memoryTokenManager{tokens: map[string]struct {
		token     string
		expiresAt time.Time
	}{}}
}

func (m *memoryTokenManager) GetToken(appKey string) (string, time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.tokens[appKey]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", time.Time{}, false
	}
	return entry.token, entry.expiresAt, true
}

func (m *memoryTokenManager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[appKey] = struct {
		token     string
		expiresAt time.Time
	}{token: token, expiresAt: expiresAt}
	return nil
}

func (m *memoryTokenManager) DeleteToken(appKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, appKey)
	return nil
}

func (m *memoryTokenManager) WaitForAuth(string) {}

func TestAuthenticate_FormBodyAndTokenCache(t *testing.T) {
	t.Parallel()

	var authCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathOAuthToken {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		authCalls++
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
			t.Fatalf("content-type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Fatalf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "client-secret" {
			t.Fatalf("unexpected credentials: %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "token-1", TokenType: "Bearer", ExpiresIn: 3600})
	}))
	defer srv.Close()

	c := NewClientWithTokenManager(false, newMemoryTokenManager())
	c.SetBaseURL(srv.URL)

	token, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "client-id", AppSecret: "client-secret"})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if token.AccessToken != "token-1" {
		t.Fatalf("access token = %q", token.AccessToken)
	}

	token, err = c.Authenticate(context.Background(), broker.Credentials{AppKey: "client-id", AppSecret: "client-secret"})
	if err != nil {
		t.Fatalf("Authenticate() cached error = %v", err)
	}
	if token.AccessToken != "token-1" {
		t.Fatalf("cached access token = %q", token.AccessToken)
	}
	if authCalls != 1 {
		t.Fatalf("auth calls = %d, want 1", authCalls)
	}
}

func TestGetPrices_RefreshesTokenAfterUnauthorized(t *testing.T) {
	var authCalls int
	var priceCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			authCalls++
			token := "token-1"
			if authCalls > 1 {
				token = "token-2"
			}
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600})
		case PathPrices:
			priceCalls++
			if priceCalls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid-token","message":"expired"}}`))
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer token-2" {
				t.Fatalf("authorization = %q, want refreshed token", got)
			}
			_ = json.NewEncoder(w).Encode(apiEnvelope[[]PriceResponse]{
				Result: []PriceResponse{{Symbol: "005930", LastPrice: "72000", Currency: "KRW"}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClientWithTokenManager(false, newMemoryTokenManager())
	c.SetBaseURL(srv.URL)
	if _, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: "refresh-client", AppSecret: "secret"}); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	prices, err := c.GetPrices(context.Background(), "005930")
	if err != nil {
		t.Fatalf("GetPrices() error = %v", err)
	}
	if len(prices) != 1 || prices[0].LastPrice != "72000" {
		t.Fatalf("unexpected prices: %+v", prices)
	}
	if authCalls != 2 {
		t.Fatalf("auth calls = %d, want 2", authCalls)
	}
}

func TestGetPrices_MapsRateLimitError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathOAuthToken:
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "token", TokenType: "Bearer", ExpiresIn: 3600})
		case PathPrices:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"rate-limit-exceeded","message":"slow down"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClientWithTokenManager(false, newMemoryTokenManager())
	c.SetBaseURL(srv.URL)
	c.SetCredentials("rate-client", "secret")

	_, err := c.GetPrices(context.Background(), "005930")
	if !errors.Is(err, broker.ErrRateLimitExceeded) {
		t.Fatalf("expected ErrRateLimitExceeded, got %v", err)
	}
}

func TestRateLimitGroup(t *testing.T) {
	t.Parallel()

	if got := RateLimitGroup(http.MethodGet, PathCandles); got != "MARKET_DATA_CHART" {
		t.Fatalf("candles group = %q", got)
	}
	if got := RateLimitGroup(http.MethodPost, PathOrders); got != "ORDER" {
		t.Fatalf("orders group = %q", got)
	}
}
