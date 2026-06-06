package ls

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int64  `json:"expires_in"`
	ErrorCode        string `json:"error_code"`
	ErrorDescription string `json:"error_description"`
}

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

	tm.WaitForAuth(appKey)

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
