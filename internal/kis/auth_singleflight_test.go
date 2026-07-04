package kis

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

// lockedTokenManager wraps stubTokenManager for concurrent use.
type lockedTokenManager struct {
	mu   sync.Mutex
	stub stubTokenManager
}

func (m *lockedTokenManager) GetToken(appKey string) (string, time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stub.GetToken(appKey)
}

func (m *lockedTokenManager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Populate the cache so callers arriving after the flight hit it.
	m.stub.cachedToken = token
	m.stub.cachedExpires = expiresAt
	m.stub.hasCached = true
	return m.stub.SetToken(appKey, token, expiresAt)
}

func (m *lockedTokenManager) DeleteToken(appKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stub.DeleteToken(appKey)
}

func (m *lockedTokenManager) WaitForAuth(appKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.WaitForAuth(appKey)
}

func (m *lockedTokenManager) waitCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stub.waitCalls
}

func TestAuthenticate_ConcurrentCallersShareSingleTokenCall(t *testing.T) {
	var authCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"herd-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer ts.Close()

	tm := &lockedTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.baseURL = ts.URL

	const callers = 20
	var wg sync.WaitGroup
	errs := make([]error, callers)
	tokens := make([]string, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := c.Authenticate(context.Background(), broker.Credentials{
				AppKey:    "herd-app-key",
				AppSecret: "app-secret",
			})
			errs[i] = err
			if tok != nil {
				tokens[i] = tok.AccessToken
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d error: %v", i, errs[i])
		}
		if tokens[i] != "herd-token" {
			t.Fatalf("caller %d token = %q, want herd-token", i, tokens[i])
		}
	}
	if got := authCalls.Load(); got != 1 {
		t.Fatalf("auth HTTP calls = %d, want 1", got)
	}
	if got := tm.waitCalls(); got != 1 {
		t.Fatalf("WaitForAuth calls = %d, want 1", got)
	}
}

func TestAuthenticate_CanceledCallerDoesNotPoisonRefresh(t *testing.T) {
	release := make(chan struct{})
	var authCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"survivor-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer ts.Close()

	tm := &lockedTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.baseURL = ts.URL

	creds := broker.Credentials{AppKey: "cancel-app-key", AppSecret: "app-secret"}

	// First caller starts the flight, then gets canceled mid-refresh.
	ctx, cancel := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := c.Authenticate(ctx, creds)
		firstErr <- err
	}()

	// Second caller joins the same flight.
	secondDone := make(chan struct{})
	var secondTok *broker.Token
	var secondErr error
	go func() {
		defer close(secondDone)
		// Give the first caller time to start the flight.
		time.Sleep(50 * time.Millisecond)
		secondTok, secondErr = c.Authenticate(context.Background(), creds)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller error = %v, want context.Canceled", err)
	}

	close(release)
	<-secondDone
	if secondErr != nil {
		t.Fatalf("second caller error: %v", secondErr)
	}
	if secondTok.AccessToken != "survivor-token" {
		t.Fatalf("second caller token = %q, want survivor-token", secondTok.AccessToken)
	}
	if got := authCalls.Load(); got != 1 {
		t.Fatalf("auth HTTP calls = %d, want 1", got)
	}
}
