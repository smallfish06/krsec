package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

type TokenResponse struct {
	AccessToken           string `json:"access_token"`
	AccessTokenExpired    string `json:"access_token_token_expired"`
	TokenType             string `json:"token_type"`
	ExpiresIn             int    `json:"expires_in"`
	AccessTokenExpiresStr string `json:"access_token_expires"`
}

// authFlight collapses concurrent token issuance per appKey so a burst of
// requests hitting an expired token results in a single /oauth2/tokenP call
// instead of queueing one-per-minute on the auth rate limiter.
var authFlight singleflight.Group

// issueTimeout bounds the HTTP round trip of a detached token issuance. The
// wait on the issuance limiter (up to 60s) happens before it starts.
const issueTimeout = 90 * time.Second

func (c *Client) Authenticate(ctx context.Context, creds broker.Credentials) (*broker.Token, error) {
	appKey := strings.TrimSpace(creds.AppKey)
	appSecret := strings.TrimSpace(creds.AppSecret)
	if appKey == "" || appSecret == "" {
		return nil, broker.ErrInvalidCredentials
	}
	creds.AppKey = appKey
	creds.AppSecret = appSecret
	c.SetCredentials(appKey, appSecret)

	// Check token manager first (shared cache across clients)
	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if token, expiresAt, ok := tm.GetToken(appKey); ok {
		c.setToken(token, expiresAt)
		return &broker.Token{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresAt:   expiresAt,
		}, nil
	}

	// Issue at most one token per appKey at a time. The issuance runs on a
	// detached context so one canceled caller cannot poison the refresh for
	// every other request waiting on it.
	ch := authFlight.DoChan(appKey, func() (any, error) {
		return issueToken(tm, c.httpClient, c.baseURL, creds)
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

// issueToken requests a fresh token from KIS and stores it in the manager.
func issueToken(tm tokencache.Manager, httpClient *http.Client, baseURL string, creds broker.Credentials) (*broker.Token, error) {
	// Another flight may have refreshed while we were queued.
	if token, expiresAt, ok := tm.GetToken(creds.AppKey); ok {
		return &broker.Token{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresAt:   expiresAt,
		}, nil
	}

	// Apply token issuance rate limit (1/minute per appkey)
	tm.WaitForAuth(creds.AppKey)

	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()

	reqBody := map[string]string{
		"grant_type": "client_credentials",
		"appkey":     creds.AppKey,
		"appsecret":  creds.AppSecret,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := baseURL + "/oauth2/tokenP"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("%w: HTTP %d: %s", broker.ErrUnauthorized, resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if strings.TrimSpace(tokenResp.AccessToken) == "" || tokenResp.ExpiresIn <= 0 {
		return nil, fmt.Errorf("%w: KIS token response carried no usable token", broker.ErrUnauthorized)
	}
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	if err := tm.SetToken(creds.AppKey, tokenResp.AccessToken, expiresAt); err != nil {
		// The token is still returned and held in memory by the caller.
		slog.Warn("failed to persist KIS token", "error", err)
	}

	return &broker.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresAt:   expiresAt,
	}, nil
}
