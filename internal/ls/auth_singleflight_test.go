package ls

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

// lockedTokenManager wraps memoryTokenManager for concurrent use.
type lockedTokenManager struct {
	mu   sync.Mutex
	stub memoryTokenManager
}

func (m *lockedTokenManager) GetToken(appKey string) (string, time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stub.GetToken(appKey)
}

func (m *lockedTokenManager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		if r.URL.Path != PathOAuthToken {
			http.NotFound(w, r)
			return
		}
		authCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"herd-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer ts.Close()

	tm := &lockedTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.SetBaseURL(ts.URL)

	const callers = 20
	var wg sync.WaitGroup
	errs := make([]error, callers)
	tokens := make([]string, callers)
	for i := range callers {
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

	for i := range callers {
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
	c.SetBaseURL(ts.URL)

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

// A caller that misses the cache but whose request context is already done
// must not leave a handler goroutine parked on the auth limiter: the wait
// happens inside the detached flight, and the caller returns as soon as its
// own context ends.
func TestAuthenticate_ReturnsPromptlyWhenCallerContextEnds(t *testing.T) {
	blockAuth := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-blockAuth
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"late-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer ts.Close()
	defer close(blockAuth)

	tm := &lockedTokenManager{}
	c := NewClientWithTokenManager(false, tm)
	c.SetBaseURL(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Authenticate(ctx, broker.Credentials{AppKey: "prompt-app-key", AppSecret: "app-secret"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Authenticate blocked %v after caller context ended", elapsed)
	}
}
