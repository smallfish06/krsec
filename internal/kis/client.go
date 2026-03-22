package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/ratelimit"
	"github.com/smallfish06/krsec/pkg/broker"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

const (
	// BaseURLReal is the production base URL
	BaseURLReal = "https://openapi.koreainvestment.com:9443"
	// BaseURLSandbox is the sandbox base URL
	BaseURLSandbox = "https://openapivts.koreainvestment.com:29443"
)

// Client is the KIS HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
	appKey     string
	appSecret  string

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time

	apiLimiter   *ratelimit.Limiter
	tokenManager tokencache.Manager
}

// NewClientWithTokenManager creates a new KIS client with an injected token manager.
// When tokenManager is nil, the global default manager is used.
func NewClientWithTokenManager(sandbox bool, tokenManager tokencache.Manager) *Client {
	baseURL := BaseURLReal
	if sandbox {
		baseURL = BaseURLSandbox
	}
	if tokenManager == nil {
		tokenManager = GetTokenManager()
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiLimiter:   ratelimit.New(broker.CodeKIS, 15, 3), // 15 req/s, burst 3
		tokenManager: tokenManager,
	}
}

// Name returns the broker name
func (c *Client) Name() string {
	return broker.NameKIS
}

// SetCredentials sets the app key and secret
func (c *Client) SetCredentials(appKey, appSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appKey = appKey
	c.appSecret = appSecret
}

// SetToken sets the access token
func (c *Client) setToken(token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessToken = token
	c.expiresAt = expiresAt
}

// GetToken returns the current access token
func (c *Client) getToken() (string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessToken, c.expiresAt
}

// IsTokenValid checks if the current token is valid.
// It checks the injected token manager first, then falls back to local token.
func (c *Client) isTokenValid() bool {
	c.mu.RLock()
	appKey := c.appKey
	c.mu.RUnlock()

	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}

	// Check token manager first
	if appKey != "" {
		if token, expiresAt, ok := tm.GetToken(appKey); ok {
			// Update local cache if different
			localToken, _ := c.getToken()
			if localToken != token {
				c.setToken(token, expiresAt)
			}
			return true
		}
	}

	// Fall back to local token check
	_, expiresAt := c.getToken()
	return time.Now().Before(expiresAt.Add(-5 * time.Minute)) // 5분 여유
}

// doRequest performs an HTTP request with authentication headers
func (c *Client) doRequest(ctx context.Context, method, path string, trID string, body interface{}, result interface{}) error {
	// Apply rate limiting
	if err := c.apiLimiter.Wait(ctx); err != nil {
		return err
	}

	// Check token validity
	if !c.isTokenValid() {
		c.mu.RLock()
		creds := broker.Credentials{
			AppKey:    c.appKey,
			AppSecret: c.appSecret,
		}
		c.mu.RUnlock()

		if _, err := c.Authenticate(ctx, creds); err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
	}

	url := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	log.Printf("DEBUG: %s %s (tr_id: %s)", method, url, trID)
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	token, _ := c.getToken()
	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("appkey", c.appKey)
	req.Header.Set("appsecret", c.appSecret)
	c.mu.RUnlock()

	if trID != "" {
		req.Header.Set("tr_id", trID)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%w: HTTP %d: %s", broker.ErrServerError, resp.StatusCode, string(bodyBytes))
		}
		if resp.StatusCode == 429 {
			return fmt.Errorf("%w: HTTP %d: %s", broker.ErrRateLimitExceeded, resp.StatusCode, string(bodyBytes))
		}
		return fmt.Errorf("%w: HTTP %d: %s", broker.ErrUpstreamBadRequest, resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func encodeQuery(basePath string, values url.Values) string {
	return basePath + "?" + values.Encode()
}

func encodeQueryWithFields(basePath string, fields map[string]string) string {
	if len(fields) == 0 {
		return basePath
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	q := url.Values{}
	for _, k := range keys {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		q.Set(key, strings.TrimSpace(fields[k]))
	}
	if len(q) == 0 {
		return basePath
	}
	return encodeQuery(basePath, q)
}

// CallDocumentedEndpointInto calls a documented KIS endpoint and decodes into result.
// result should be a pointer to a struct response type.
func (c *Client) CallDocumentedEndpointInto(
	ctx context.Context,
	method string,
	path string,
	trID string,
	fields map[string]string,
	result interface{},
) error {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = http.MethodGet
	}

	if m == http.MethodGet {
		path = encodeQueryWithFields(path, fields)
		if err := c.doRequest(ctx, m, path, trID, nil, result); err != nil {
			return err
		}
		return checkEndpointResult(result)
	}

	body := make(map[string]string, len(fields))
	for k, v := range fields {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		body[key] = strings.TrimSpace(v)
	}

	if err := c.doRequest(ctx, m, path, trID, body, result); err != nil {
		return err
	}
	return checkEndpointResult(result)
}

// ResolveTRID picks the environment-appropriate TR_ID from documented real/virtual IDs.
// It returns empty string when no usable TR_ID exists.
func (c *Client) ResolveTRID(realTRID, virtualTRID string) string {
	normalize := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		// KIS docs use this sentinel for endpoints that do not support paper trading.
		if strings.Contains(v, "모의투자 미지원") {
			return ""
		}
		return v
	}

	real := normalize(realTRID)
	virtual := normalize(virtualTRID)

	if c.baseURL == BaseURLSandbox {
		if virtual != "" {
			return virtual
		}
		return real
	}
	if real != "" {
		return real
	}
	return virtual
}

func checkEndpointResult(result interface{}) error {
	switch v := result.(type) {
	case nil:
		return nil
	case kisspecs.DocumentedEndpointResponse:
		if !v.IsSuccess() {
			return fmt.Errorf("KIS API error: %s (%s)", v.GetMsg1(), v.GetMsgCode())
		}
		return nil
	default:
		// Unknown response shape: do not enforce semantic success check here.
		return nil
	}
}
