package kiwoom

import (
	"bytes"
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

// authFlight collapses concurrent token issuance per appKey so a burst of
// requests hitting an expired token results in a single token call instead
// of queueing one-per-minute on the auth rate limiter.
var authFlight singleflight.Group

// issueTimeout bounds a detached token issuance: up to one auth-limiter
// window (60s) plus the HTTP round trip.
const issueTimeout = 90 * time.Second

// TokenResponse is Kiwoom OAuth token response.
type TokenResponse struct {
	ExpiresDT  string      `json:"expires_dt"`
	TokenType  string      `json:"token_type"`
	Token      string      `json:"token"`
	ReturnCode interface{} `json:"return_code"`
	ReturnMsg  string      `json:"return_msg"`
}

// Authenticate issues or reuses an OAuth token.
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
	ch := authFlight.DoChan(appKey, func() (interface{}, error) {
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

// issueToken requests a fresh token from Kiwoom and stores it in the manager.
func (c *Client) issueToken(tm tokencache.Manager, appKey, appSecret string) (*broker.Token, error) {
	// Another flight may have refreshed while we were queued.
	if token, expiresAt, ok := tm.GetToken(appKey); ok {
		return &broker.Token{AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
	}

	tm.WaitForAuth(appKey)

	ctx, cancel := context.WithTimeout(context.Background(), issueTimeout)
	defer cancel()

	reqBody := map[string]string{
		"grant_type": "client_credentials",
		"appkey":     appKey,
		"secretkey":  appSecret,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal auth request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do auth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read auth response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if code, msg, ok := parseErrorPayload(bodyBytes); ok {
			if code == 0 {
				code = resp.StatusCode
			}
			return nil, wrapAuthError(code, msg)
		}
		return nil, wrapAuthError(resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var tr TokenResponse
	if err := json.Unmarshal(bodyBytes, &tr); err != nil {
		return nil, fmt.Errorf("decode auth response: %w", err)
	}
	if code := parseReturnCode(tr.ReturnCode); code != 0 {
		return nil, wrapAuthError(code, tr.ReturnMsg)
	}
	if strings.TrimSpace(tr.Token) == "" {
		return nil, fmt.Errorf("auth response missing token")
	}

	expiresAt := parseExpiry(tr.ExpiresDT)
	c.setToken(tr.Token, expiresAt)
	if err := tm.SetToken(appKey, tr.Token, expiresAt); err != nil {
		// cache write failure should not fail auth flow
		c.logger.Warn("failed to persist kiwoom token", "error", err)
	}

	typeVal := strings.TrimSpace(tr.TokenType)
	if typeVal == "" {
		typeVal = "bearer"
	}

	return &broker.Token{
		AccessToken: tr.Token,
		TokenType:   typeVal,
		ExpiresAt:   expiresAt,
	}, nil
}

func parseExpiry(expiresDT string) time.Time {
	expiresDT = strings.TrimSpace(expiresDT)
	if expiresDT != "" {
		if t, err := time.ParseInLocation("20060102150405", expiresDT, time.Local); err == nil {
			return t
		}
	}
	// Fallback when docs/sample omits/changes expiry format.
	return time.Now().Add(23 * time.Hour)
}
