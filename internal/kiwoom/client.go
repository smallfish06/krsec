package kiwoom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/ratelimit"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

const (
	// BaseURLReal is Kiwoom production REST domain.
	BaseURLReal = "https://api.kiwoom.com"
	// BaseURLSandbox is Kiwoom mock REST domain.
	BaseURLSandbox = "https://mockapi.kiwoom.com"
)

// Client is a Kiwoom HTTP client with token caching and API-id routing.
type Client struct {
	baseURL    string
	httpClient *http.Client

	mu          sync.RWMutex
	appKey      string
	appSecret   string
	accessToken string
	expiresAt   time.Time

	apiLimiter   *ratelimit.Limiter
	tokenManager tokencache.Manager
	logger       *slog.Logger
}

// callOptions controls optional Kiwoom continuation headers.
type callOptions struct {
	ContYN  string
	NextKey string
	Headers map[string]string
}

func cloneBody(body map[string]any) map[string]any {
	if body == nil {
		return map[string]any{}
	}
	return maps.Clone(body)
}

// NewClientWithTokenManager creates a client with injected token manager.
func NewClientWithTokenManager(sandbox bool, tm tokencache.Manager) *Client {
	baseURL := BaseURLReal
	if sandbox {
		baseURL = BaseURLSandbox
	}
	if tm == nil {
		tm = GetTokenManager()
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiLimiter:   ratelimit.New(broker.CodeKiwoom, 8, 2), // 8 req/s, burst 2
		tokenManager: tm,
		logger:       slog.Default(),
	}
}

// SetLogger sets the logger for the client.
func (c *Client) SetLogger(l *slog.Logger) {
	if l != nil {
		c.logger = l
	}
}

// Name returns broker name.
func (c *Client) Name() string {
	return broker.NameKiwoom
}

// SetCredentials sets app credentials on this client.
func (c *Client) SetCredentials(appKey, appSecret string) {
	c.mu.Lock()
	c.appKey = strings.TrimSpace(appKey)
	c.appSecret = strings.TrimSpace(appSecret)
	c.mu.Unlock()
}

// SetBaseURL overrides the API base URL.
// Primarily useful for tests or private/proxy deployments.
func (c *Client) SetBaseURL(baseURL string) {
	c.mu.Lock()
	c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	c.mu.Unlock()
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

func (c *Client) getCredentials() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.appKey, c.appSecret
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

func documentedEndpointMethod(path, apiID string) (string, error) {
	spec, ok := kiwoomspecs.LookupDocumentedEndpointSpec(path, apiID)
	if !ok {
		return "", fmt.Errorf("missing documented endpoint spec for %s/%s", strings.TrimSpace(path), strings.TrimSpace(apiID))
	}
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodPost
	}
	return method, nil
}

// CallDocumentedEndpoint executes a documented Kiwoom REST endpoint.
// It returns generated response type for known documented path/api_id.
func (c *Client) CallDocumentedEndpoint(
	ctx context.Context,
	apiID, path string,
	body any,
	allowedCodes ...int,
) (any, error) {
	method, err := documentedEndpointMethod(path, apiID)
	if err != nil {
		return nil, err
	}
	if err := validateDocumentedRequestBody(body); err != nil {
		return nil, err
	}

	bodyBytes, err := c.doRequest(ctx, method, apiID, path, normalizeRequestBody(body), callOptions{})
	if err != nil {
		return nil, err
	}
	code, msg := decodeReturnStatus(bodyBytes)
	if code != 0 && !containsCode(allowedCodes, code) {
		return nil, wrapCallError(apiID, code, msg)
	}
	resp := kiwoomspecs.NewDocumentedEndpointResponse(strings.TrimSpace(path), strings.TrimSpace(apiID))
	if resp == nil {
		out := make(map[string]any)
		if len(bytes.TrimSpace(bodyBytes)) == 0 {
			return out, nil
		}
		if err := json.Unmarshal(bodyBytes, &out); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		return out, nil
	}
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return resp, nil
	}
	if err := json.Unmarshal(bodyBytes, resp); err != nil {
		// Documented schema can diverge from runtime response shape.
		out := make(map[string]any)
		if err := json.Unmarshal(bodyBytes, &out); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		return out, nil
	}
	return resp, nil
}

func containsCode(codes []int, target int) bool {
	return slices.Contains(codes, target)
}

func validateDocumentedRequestBody(body any) error {
	switch body.(type) {
	case map[string]any, map[string]string:
		return fmt.Errorf("documented endpoint request must use generated request type (map body is not allowed)")
	default:
		return nil
	}
}

func decodeReturnStatus(bodyBytes []byte) (int, string) {
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return 0, ""
	}
	var payload struct {
		ReturnCode any    `json:"return_code"`
		ReturnMsg  string `json:"return_msg"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return 0, ""
	}
	return parseReturnCode(payload.ReturnCode), strings.TrimSpace(payload.ReturnMsg)
}

func responseBodyMap(v any) (map[string]any, error) {
	switch t := v.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return cloneBody(t), nil
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = val
		}
		return out, nil
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal response: %w", err)
		}
		out := make(map[string]any)
		if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return out, nil
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		return out, nil
	}
}

func bindResponseObject(v any, out any) error {
	if out == nil {
		return fmt.Errorf("response target is nil")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, apiID, path string, body any, opts callOptions) ([]byte, error) {
	if err := c.apiLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	if !c.isTokenValid() {
		appKey, appSecret := c.getCredentials()
		if appKey == "" || appSecret == "" {
			return nil, fmt.Errorf("missing credentials for token refresh")
		}
		if _, err := c.Authenticate(ctx, broker.Credentials{AppKey: appKey, AppSecret: appSecret}); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	path = strings.TrimSpace(path)
	apiID = strings.TrimSpace(apiID)
	if path == "" || apiID == "" {
		return nil, fmt.Errorf("invalid endpoint arguments")
	}
	requestURL := strings.TrimRight(c.baseURL, "/") + path
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}

	req, err := c.newHTTPRequest(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}

	token, _ := c.getToken()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("api-id", apiID)

	if cont := strings.TrimSpace(opts.ContYN); cont != "" {
		req.Header.Set("cont-yn", cont)
	}
	if next := strings.TrimSpace(opts.NextKey); next != "" {
		req.Header.Set("next-key", next)
	}
	for k, v := range opts.Headers {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		req.Header.Set(key, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if code, msg, ok := parseErrorPayload(bodyBytes); ok {
			if code == 0 {
				code = resp.StatusCode
			}
			return nil, wrapCallError(apiID, code, msg)
		}
		msg := strings.TrimSpace(string(bodyBytes))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, wrapCallError(apiID, resp.StatusCode, msg)
	}
	return bodyBytes, nil
}

func (c *Client) newHTTPRequest(ctx context.Context, method string, requestURL string, body any) (*http.Request, error) {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		bodyMap, err := bodyToMap(body)
		if err != nil {
			return nil, fmt.Errorf("encode request query: %w", err)
		}
		u, err := neturl.Parse(requestURL)
		if err != nil {
			return nil, fmt.Errorf("parse request URL: %w", err)
		}
		q := u.Query()
		for k, v := range bodyMap {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			q.Set(key, strings.TrimSpace(toString(v)))
		}
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		return req, nil
	default:
		payload, err := json.Marshal(normalizeRequestBody(body))
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		return req, nil
	}
}

func normalizeRequestBody(body any) any {
	if body == nil {
		return map[string]any{}
	}
	return body
}

func bodyToMap(body any) (map[string]any, error) {
	switch t := body.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return cloneBody(t), nil
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = v
		}
		return out, nil
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return map[string]any{}, nil
		}
		out := make(map[string]any)
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func parseReturnCode(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return int(n)
		}
		f, err := t.Float64()
		if err == nil {
			return int(f)
		}
		return 0
	default:
		s := strings.TrimSpace(toString(v))
		if s == "" {
			return 0
		}
		code, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return code
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
