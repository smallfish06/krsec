package kis

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

func TestAuthenticateRejectsMissingCredentialsBeforeTokenLookup(t *testing.T) {
	tm := &stubTokenManager{}
	c := NewClientWithTokenManager(false, tm)

	for _, creds := range []broker.Credentials{
		{},
		{AppKey: "app-key"},
		{AppSecret: "app-secret"},
		{AppKey: "  ", AppSecret: "app-secret"},
	} {
		if _, err := c.Authenticate(context.Background(), creds); !errors.Is(err, broker.ErrInvalidCredentials) {
			t.Fatalf("Authenticate(%+v) error = %v, want ErrInvalidCredentials", creds, err)
		}
	}
	if tm.getCalls != 0 || tm.waitCalls != 0 || tm.setCalls != 0 {
		t.Fatalf("token manager called for invalid credentials: get=%d wait=%d set=%d", tm.getCalls, tm.waitCalls, tm.setCalls)
	}
}

type stubTokenManager struct {
	cachedToken   string
	cachedExpires time.Time
	hasCached     bool

	getCalls  int
	waitCalls int
	setCalls  int

	lastGetAppKey string
	lastSetAppKey string
	lastSetToken  string
	lastSetExpiry time.Time

	setErr    error
	delCalls  int
	delAppKey string
}

func (m *stubTokenManager) GetToken(appKey string) (string, time.Time, bool) {
	m.getCalls++
	m.lastGetAppKey = appKey
	return m.cachedToken, m.cachedExpires, m.hasCached
}

func (m *stubTokenManager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.setCalls++
	m.lastSetAppKey = appKey
	m.lastSetToken = token
	m.lastSetExpiry = expiresAt
	return m.setErr
}

func (m *stubTokenManager) DeleteToken(appKey string) error {
	m.delCalls++
	m.delAppKey = appKey
	m.hasCached = false
	m.cachedToken = ""
	return nil
}

func (m *stubTokenManager) WaitForAuth(string) {
	m.waitCalls++
}

func TestIsTokenValid_UsesInjectedTokenManager(t *testing.T) {
	tm := &stubTokenManager{
		cachedToken:   "cached-token",
		cachedExpires: time.Now().Add(time.Hour),
		hasCached:     true,
	}

	c := NewClientWithTokenManager(false, tm)
	c.SetCredentials("app-key", "app-secret")

	if !c.isTokenValid() {
		t.Fatal("expected token to be valid from injected token manager")
	}

	if tm.getCalls != 1 {
		t.Fatalf("GetToken calls = %d, want 1", tm.getCalls)
	}
	if tm.lastGetAppKey != "app-key" {
		t.Fatalf("lastGetAppKey = %q, want app-key", tm.lastGetAppKey)
	}

	token, _ := c.getToken()
	if token != "cached-token" {
		t.Fatalf("local token = %q, want cached-token", token)
	}
}

func TestAuthenticate_UsesInjectedCachedToken(t *testing.T) {
	var authCalls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	tm := &stubTokenManager{
		cachedToken:   "cached-token",
		cachedExpires: time.Now().Add(time.Hour),
		hasCached:     true,
	}

	c := NewClientWithTokenManager(false, tm)
	c.baseURL = ts.URL

	tok, err := c.Authenticate(context.Background(), broker.Credentials{
		AppKey:    "app-key",
		AppSecret: "app-secret",
	})
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if tok.AccessToken != "cached-token" {
		t.Fatalf("access token = %q, want cached-token", tok.AccessToken)
	}
	if authCalls != 0 {
		t.Fatalf("unexpected auth HTTP calls: %d", authCalls)
	}
	if tm.waitCalls != 0 {
		t.Fatalf("WaitForAuth calls = %d, want 0", tm.waitCalls)
	}
	if tm.setCalls != 0 {
		t.Fatalf("SetToken calls = %d, want 0", tm.setCalls)
	}
}

func TestAuthenticate_StoresTokenInInjectedTokenManager(t *testing.T) {
	var authCalls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/tokenP" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		authCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-token","token_type":"Bearer","expires_in":3600,"access_token_token_expired":"","access_token_expires":""}`))
	}))
	defer ts.Close()

	tm := &stubTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.baseURL = ts.URL

	tok, err := c.Authenticate(context.Background(), broker.Credentials{
		AppKey:    "app-key",
		AppSecret: "app-secret",
	})
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if tok.AccessToken != "new-token" {
		t.Fatalf("access token = %q, want new-token", tok.AccessToken)
	}
	if authCalls != 1 {
		t.Fatalf("auth HTTP calls = %d, want 1", authCalls)
	}
	if tm.waitCalls != 1 {
		t.Fatalf("WaitForAuth calls = %d, want 1", tm.waitCalls)
	}
	if tm.setCalls != 1 {
		t.Fatalf("SetToken calls = %d, want 1", tm.setCalls)
	}
	if tm.lastSetAppKey != "app-key" {
		t.Fatalf("lastSetAppKey = %q, want app-key", tm.lastSetAppKey)
	}
	if tm.lastSetToken != "new-token" {
		t.Fatalf("lastSetToken = %q, want new-token", tm.lastSetToken)
	}
	if time.Until(tm.lastSetExpiry) < 50*time.Minute {
		t.Fatalf("unexpected expiry window: %s", tm.lastSetExpiry)
	}
}
