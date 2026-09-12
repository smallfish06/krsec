package ls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// lockedTokenManager wraps memoryTokenManager for concurrent use.
type lockedTokenManager struct {
	mu   sync.Mutex
	stub memoryTokenManager
}

type notifyingAuthManager struct {
	tokencache.Manager
	key    string
	cached chan struct{}
}

func TestStaleAutomaticRefreshCannotReselectPreviousAccount(t *testing.T) {
	c := NewClientWithTokenManager(false, &lockedTokenManager{})
	c.SetCredentials("previous", "previous-secret")
	key, secret := c.getCredentials() // A request captured these before the switch.
	c.SetCredentials("current", "current-secret")
	c.setToken("current-token", time.Now().Add(time.Hour))
	if _, err := c.authenticateSelectedCredentials(context.Background(), key, secret); !errors.Is(err, broker.ErrUnauthorized) {
		t.Fatalf("stale automatic refresh was not rejected: %v", err)
	}
	if current, _ := c.getCredentials(); current != "current" {
		t.Fatal("automatic refresh changed account selection")
	}
	if token, _ := c.getToken(); token != "current-token" {
		t.Fatal("automatic refresh replaced the active account token")
	}
}

func (m *notifyingAuthManager) SetToken(key, token string, expires time.Time) error {
	err := m.Manager.SetToken(key, token, expires)
	if key == m.key {
		close(m.cached)
	}
	return err
}

func TestAuthenticateOldIssuanceCannotReplaceNewAccountToken(t *testing.T) {
	for _, cancelOld := range []bool{false, true} {
		t.Run(map[bool]string{false: "superseded", true: "canceled"}[cancelOld], func(t *testing.T) {
			oldKey := t.Name() + "-old"
			newKey := t.Name() + "-new"
			started, release := make(chan struct{}), make(chan struct{})
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				key := r.Form.Get("appkey")
				if key == oldKey {
					close(started)
					<-release
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-" + key, "token_type": "Bearer", "expires_in": 3600})
			}))
			defer ts.Close()
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			tm := &notifyingAuthManager{Manager: NewFileTokenManagerWithDir(t.TempDir()), key: oldKey, cached: make(chan struct{})}
			c := NewClientWithTokenManager(false, tm)
			c.SetBaseURL(ts.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			oldDone := make(chan error, 1)
			go func() {
				_, err := c.Authenticate(ctx, broker.Credentials{AppKey: oldKey, AppSecret: "local-test-secret"})
				oldDone <- err
			}()
			<-started
			if cancelOld {
				cancel()
				if err := <-oldDone; !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled auth: %v", err)
				}
			}
			newToken, err := c.Authenticate(context.Background(), broker.Credentials{AppKey: newKey, AppSecret: "local-test-secret"})
			if err != nil {
				t.Fatal(err)
			}
			releaseOnce.Do(func() { close(release) })
			<-tm.cached
			if !cancelOld {
				if err := <-oldDone; !errors.Is(err, broker.ErrUnauthorized) {
					t.Fatalf("superseded authentication succeeded: %v", err)
				}
			}
			if token, _ := c.getToken(); token != newToken.AccessToken {
				t.Fatalf("old account token replaced active account token: %q", token)
			}
			if !c.isTokenValid() {
				t.Fatal("new account token invalidated")
			}
			if token, _ := c.getToken(); token != newToken.AccessToken {
				t.Fatal("cache lookup restored another account's token")
			}
		})
	}
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
