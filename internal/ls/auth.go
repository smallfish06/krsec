package ls

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int64  `json:"expires_in"`
	ErrorCode        string `json:"error_code"`
	ErrorDescription string `json:"error_description"`
}

// authFlight collapses concurrent token issuance per appKey so a burst of
// requests hitting an expired or revoked token results in a single token call
// instead of queueing one-per-minute on the auth rate limiter. Without this,
// every LS request that missed the token cache blocked serially on the
// uncancellable auth limiter (the k-th waiter for k minutes) while the HTTP
// client had long since given up, and the parked handler goroutines grew until
// the process was OOM-killed.
var authFlight singleflight.Group

// issueTimeout bounds a detached token issuance: up to one auth-limiter
// window (60s) plus the HTTP round trip.
const issueTimeout = 90 * time.Second

// Authenticate issues or reuses an LS OAuth token.
func (c *Client) Authenticate(ctx context.Context, creds broker.Credentials) (*broker.Token, error) {
	appKey := strings.TrimSpace(creds.AppKey)
	appSecret := strings.TrimSpace(creds.AppSecret)
	if appKey == "" || appSecret == "" {
		return nil, broker.ErrInvalidCredentials
	}
	c.SetCredentials(appKey, appSecret)

	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if token, expiresAt, ok := tm.GetToken(appKey); ok {
		c.setToken(token, expiresAt)
		return &broker.Token{AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
	}

	// Issue at most one token per appKey at a time. The issuance runs on a
	// detached context so one canceled caller cannot poison the refresh for
	// every other request waiting on it.
	ch := authFlight.DoChan(appKey, func() (any, error) {
		return c.issueToken(tm, appKey, appSecret)
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		token := res.Val.(*broker.Token)
		c.setToken(token.AccessToken, token.ExpiresAt)
		return token, nil
	case <-ctx.Done():
		// The in-flight issuance keeps running and caches its result for
		// subsequent callers.
		return nil, ctx.Err()
	}
}

// issueToken requests a fresh token from LS and stores it in the manager.
func (c *Client) issueToken(tm tokencache.Manager, appKey, appSecret string) (*broker.Token, error) {
	// Another flight may have refreshed while we were queued.
	if token, expiresAt, ok := tm.GetToken(appKey); ok {
		return &broker.Token{AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
	}

	tm.WaitForAuth(appKey)

	// The limiter wait above may have taken up to a minute; a concurrent flight
	// on another client sharing the same manager may have issued meanwhile.
	if token, expiresAt, ok := tm.GetToken(appKey); ok {
		return &broker.Token{AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()

	baseURL, _ := c.urls()
	endpoint := strings.TrimRight(baseURL, "/") + PathOAuthToken
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(tokenRequestValues(appKey, appSecret).Encode()))
	if err != nil {
		return nil, fmt.Errorf("create LS auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do LS auth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read LS auth response: %w", err)
	}

	var tr tokenResponse
	if len(strings.TrimSpace(string(bodyBytes))) > 0 {
		if err := json.Unmarshal(bodyBytes, &tr); err != nil {
			return nil, fmt.Errorf("decode LS auth response: %w", err)
		}
	}
	if resp.StatusCode >= 400 {
		if tr.ErrorDescription != "" || tr.ErrorCode != "" {
			return nil, fmt.Errorf("%w: LS %s %s", broker.ErrInvalidCredentials, tr.ErrorCode, tr.ErrorDescription)
		}
		return nil, mapHTTPError(resp.StatusCode, bodyBytes)
	}
	if tr.ErrorCode != "" {
		return nil, fmt.Errorf("%w: LS %s %s", broker.ErrInvalidCredentials, tr.ErrorCode, tr.ErrorDescription)
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return nil, fmt.Errorf("LS auth response missing access_token")
	}

	expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(23 * time.Hour)
	}
	tokenType := strings.TrimSpace(tr.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	c.setToken(tr.AccessToken, expiresAt)
	if err := tm.SetToken(appKey, tr.AccessToken, expiresAt); err != nil {
		c.logger.Warn("failed to persist LS token", "error", err)
	}

	return &broker.Token{
		AccessToken: tr.AccessToken,
		TokenType:   tokenType,
		ExpiresAt:   expiresAt,
	}, nil
}
