package toss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/ratelimit"
	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
	tossspecs "github.com/smallfish06/krsec/pkg/toss/specs"
)

// Client is a Toss Securities Open API REST client.
type Client struct {
	baseURL    string
	httpClient *http.Client

	mu          sync.RWMutex
	appKey      string
	appSecret   string
	accessToken string
	expiresAt   time.Time

	tokenManager tokencache.Manager
	logger       *slog.Logger
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// NewClientWithTokenManager creates a Toss Open API client.
func NewClientWithTokenManager(sandbox bool, tm tokencache.Manager) *Client {
	if sandbox {
		// Toss currently documents only one production Open API base URL.
		slog.Default().Warn("Toss sandbox flag ignored; using production base URL")
	}
	if tm == nil {
		tm = GetTokenManager()
	}
	return &Client{
		baseURL: strings.TrimRight(BaseURLReal, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		tokenManager: tm,
		logger:       slog.Default(),
	}
}

// SetLogger sets the client logger.
func (c *Client) SetLogger(l *slog.Logger) {
	if l != nil {
		c.logger = l
	}
}

// SetBaseURL overrides the REST base URL. Tests use this for httptest servers.
func (c *Client) SetBaseURL(baseURL string) {
	c.mu.Lock()
	c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	c.mu.Unlock()
}

// Name returns broker name.
func (c *Client) Name() string {
	return broker.NameToss
}

// SetCredentials stores Toss client credentials for token refresh.
func (c *Client) SetCredentials(appKey, appSecret string) {
	c.mu.Lock()
	c.appKey = strings.TrimSpace(appKey)
	c.appSecret = strings.TrimSpace(appSecret)
	c.mu.Unlock()
}

func (c *Client) getCredentials() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.appKey, c.appSecret
}

func (c *Client) setToken(token string, expiresAt time.Time) {
	c.mu.Lock()
	c.accessToken = token
	c.expiresAt = expiresAt
	c.mu.Unlock()
}

func (c *Client) getToken() (string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessToken, c.expiresAt
}

func (c *Client) url(path string) string {
	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()
	return strings.TrimRight(baseURL, "/") + normalizePath(path)
}

func (c *Client) isTokenValid() bool {
	appKey, _ := c.getCredentials()
	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if appKey != "" {
		if token, expiresAt, ok := tm.GetToken(appKey); ok {
			cached, _ := c.getToken()
			if cached != token {
				c.setToken(token, expiresAt)
			}
			return true
		}
	}
	_, expiresAt := c.getToken()
	return time.Now().Before(expiresAt.Add(-2 * time.Minute))
}

func (c *Client) ensureToken(ctx context.Context) error {
	if c.isTokenValid() {
		return nil
	}
	appKey, appSecret := c.getCredentials()
	if appKey == "" || appSecret == "" {
		return broker.ErrInvalidCredentials
	}
	_, err := c.Authenticate(ctx, broker.Credentials{AppKey: appKey, AppSecret: appSecret})
	return err
}

// Authenticate issues or reuses a Toss OAuth token.
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
	if err := c.waitForRateLimit(ctx, http.MethodPost, PathOAuthToken); err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("client_id", appKey)
	values.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(PathOAuthToken), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create Toss auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Toss auth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Toss auth response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, mapOAuthError(resp.StatusCode, bodyBytes)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode Toss auth response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, fmt.Errorf("%w: Toss auth response missing access_token", broker.ErrServerError)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(23 * time.Hour)
	}
	tokenType := strings.TrimSpace(tokenResp.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	c.setToken(tokenResp.AccessToken, expiresAt)
	if err := tm.SetToken(appKey, tokenResp.AccessToken, expiresAt); err != nil {
		c.logger.Warn("failed to persist Toss token", "error", err)
	}

	return &broker.Token{AccessToken: tokenResp.AccessToken, TokenType: tokenType, ExpiresAt: expiresAt}, nil
}

func (c *Client) invalidateAndRefresh(ctx context.Context) error {
	appKey, _ := c.getCredentials()
	c.setToken("", time.Time{})
	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if appKey != "" {
		_ = tm.DeleteToken(appKey)
	}
	return c.ensureToken(ctx)
}

func (c *Client) callJSON(ctx context.Context, method, path, accountSeq string, query map[string]interface{}, body interface{}, result interface{}) error {
	resp, err := c.callOnce(ctx, method, path, accountSeq, query, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized && !isOrderMutation(method, path) {
		_ = resp.Body.Close()
		if err := c.invalidateAndRefresh(ctx); err != nil {
			return fmt.Errorf("token refresh after Toss 401 failed: %w", err)
		}
		resp, err = c.callOnce(ctx, method, path, accountSeq, query, body)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read Toss response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return mapUpstreamError(resp.StatusCode, bodyBytes)
	}
	if result == nil || len(bytes.TrimSpace(bodyBytes)) == 0 {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, result); err != nil {
		return fmt.Errorf("decode Toss response: %w", err)
	}
	return nil
}

func (c *Client) callOnce(ctx context.Context, method, path, accountSeq string, query map[string]interface{}, body interface{}) (*http.Response, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	path = normalizePath(path)
	if _, ok := tossspecs.LookupDocumentedEndpointSpec(method, path); !ok {
		return nil, fmt.Errorf("%w: unsupported Toss endpoint %s %s", broker.ErrUpstreamBadRequest, method, path)
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	if err := c.waitForRateLimit(ctx, method, path); err != nil {
		return nil, err
	}

	u, err := url.Parse(c.url(path))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range query {
		key := strings.TrimSpace(k)
		if key == "" || v == nil {
			continue
		}
		switch vv := v.(type) {
		case []string:
			for _, one := range vv {
				q.Add(key, strings.TrimSpace(one))
			}
		case []interface{}:
			for _, one := range vv {
				q.Add(key, fmt.Sprint(one))
			}
		default:
			q.Set(key, fmt.Sprint(vv))
		}
	}
	u.RawQuery = q.Encode()

	var bodyReader io.Reader
	if body != nil && method != http.MethodGet {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal Toss request: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create Toss request: %w", err)
	}
	token, _ := c.getToken()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(accountSeq) != "" {
		req.Header.Set("X-Tossinvest-Account", strings.TrimSpace(accountSeq))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Toss request %s %s: %w", method, path, err)
	}
	return resp, nil
}

// CallEndpoint executes a Toss endpoint and decodes the response as an untyped object.
func (c *Client) CallEndpoint(ctx context.Context, method, path, accountSeq string, query map[string]interface{}, body interface{}) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := c.callJSON(ctx, method, path, accountSeq, query, body, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

func (c *Client) waitForRateLimit(ctx context.Context, method, path string) error {
	group := RateLimitGroup(method, path)
	rps, burst := rateLimitForGroup(group)
	appKey, _ := c.getCredentials()
	if appKey == "" {
		appKey = "default"
	}
	return ratelimit.Shared("toss-"+strings.ToLower(group), rps, burst, appKey).Wait(ctx)
}

// RateLimitGroup returns the Toss rate-limit group for a documented operation.
func RateLimitGroup(method, path string) string {
	if spec, ok := tossspecs.LookupDocumentedEndpointSpec(method, path); ok && strings.TrimSpace(spec.RateLimitGroup) != "" {
		return spec.RateLimitGroup
	}
	return "MARKET_DATA"
}

func rateLimitForGroup(group string) (float64, int) {
	switch strings.ToUpper(strings.TrimSpace(group)) {
	case "AUTH":
		return 5, 1
	case "ACCOUNT":
		return 1, 1
	case "ASSET", "STOCK", "MARKET_DATA_CHART", "ORDER_HISTORY":
		return 5, 1
	case "MARKET_INFO":
		return 3, 1
	case "MARKET_DATA":
		return 10, 2
	case "ORDER", "ORDER_INFO":
		// Use the documented peak-time limit as the always-on cap.
		return 3, 1
	default:
		return 1, 1
	}
}

func isOrderMutation(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete && method != http.MethodPatch {
		return false
	}
	return strings.HasPrefix(normalizePath(path), PathOrders)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
