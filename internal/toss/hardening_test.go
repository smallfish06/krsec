package toss

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

func TestOrderPathEscapesTheOrderID(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"ord-1":         "/api/v1/orders/ord-1",
		"x?force=true":  "/api/v1/orders/x%3Fforce=true",
		"a/../accounts": "/api/v1/orders/a%2F..%2Faccounts",
	} {
		if got := orderPath(id); got != want {
			t.Errorf("orderPath(%q) = %q, want %q", id, got, want)
		}
	}
}

type lockedTokenManager struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (m *lockedTokenManager) GetToken(string) (string, time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.token, m.expiresAt, m.token != ""
}

func (m *lockedTokenManager) SetToken(_, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token, m.expiresAt = token, expiresAt
	return nil
}

func (m *lockedTokenManager) DeleteToken(string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = ""
	return nil
}

func (m *lockedTokenManager) WaitForAuth(string) {}

// Toss revokes the previous token on every issuance, so a burst of requests
// with no cached token must issue exactly one.
func TestConcurrentAuthenticateIssuesOneToken(t *testing.T) {
	var issued atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		issued.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer ts.Close()

	tm := &lockedTokenManager{}
	creds := broker.Credentials{AppKey: "single-flight-key", AppSecret: "secret"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			// Separate clients, as two accounts sharing one app key have.
			c := NewClientWithTokenManager(false, tm)
			c.SetBaseURL(ts.URL)
			if _, err := c.Authenticate(context.Background(), creds); err != nil {
				t.Errorf("Authenticate: %v", err)
			}
		})
	}
	wg.Wait()

	if got := issued.Load(); got != 1 {
		t.Fatalf("issued %d tokens, want 1: each extra issuance revokes the others", got)
	}
}
