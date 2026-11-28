package ls

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/ratelimit"
	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// Client is an LS Securities OpenAPI REST/WebSocket client.
type Client struct {
	baseURL string
	wsURL   string

	httpClient *http.Client
	limiter    *ratelimit.Limiter

	mu          sync.RWMutex
	appKey      string
	appSecret   string
	accessToken string
	expiresAt   time.Time
	macAddress  string

	tokenManager tokencache.Manager
	logger       *slog.Logger
}

type callOptions struct {
	TRCont    string
	TRContKey string
	Headers   map[string]string
}

// Continuation carries the LS response headers needed for the next page.
type Continuation struct {
	TRCont    string `json:"tr_cont,omitempty"`
	TRContKey string `json:"tr_cont_key,omitempty"`
}

// EndpointPage preserves a REST payload and its independent continuation headers.
type EndpointPage struct {
	Data map[string]any `json:"data"`
	Continuation
}

// CallEndpointPage executes one page without discarding continuation metadata.
func (c *Client) CallEndpointPage(ctx context.Context, method, path, trCD string, request any, continuation Continuation) (*EndpointPage, error) {
	cont := strings.ToUpper(strings.TrimSpace(continuation.TRCont))
	key := strings.TrimSpace(continuation.TRContKey)
	if strings.ContainsAny(key, "\r\n") {
		return nil, fmt.Errorf("%w: invalid LS continuation key", broker.ErrUpstreamBadRequest)
	}
	if (cont != "" && cont != "N" && cont != "Y") || (cont == "Y" && key == "") || (key != "" && cont != "Y") {
		return nil, fmt.Errorf("%w: continuation requires tr_cont=Y and a non-empty tr_cont_key", broker.ErrUpstreamBadRequest)
	}
	return c.callEndpointAttempt(ctx, method, path, trCD, request, callOptions{TRCont: cont, TRContKey: key}, true)
}

func NewClientWithTokenManager(sandbox bool, tm tokencache.Manager) *Client {
	baseURL := BaseURLReal
	wsURL := WebSocketURLReal
	if sandbox {
		baseURL = BaseURLSandbox
		wsURL = WebSocketURLSandbox
	}
	if tm == nil {
		tm = GetTokenManager()
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		wsURL:   wsURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Per-TR limits vary from 1 to 10 req/s. A conservative shared client
		// limit avoids accidental bursts while endpoint-specific generation is added.
		limiter:      ratelimit.New(broker.CodeLS, 5, 1),
		tokenManager: tm,
		logger:       slog.Default(),
	}
}

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

// SetWebSocketURL overrides the realtime WebSocket endpoint.
func (c *Client) SetWebSocketURL(wsURL string) {
	c.mu.Lock()
	c.wsURL = strings.TrimSpace(wsURL)
	c.mu.Unlock()
}

// SetMACAddress sets the optional mac_address header required for LS corporate accounts.
func (c *Client) SetMACAddress(macAddress string) {
	c.mu.Lock()
	c.macAddress = strings.TrimSpace(macAddress)
	c.mu.Unlock()
}

func (c *Client) Name() string {
	return broker.NameLS
}

// SetCredentials stores LS app credentials for token refresh.
func (c *Client) SetCredentials(appKey, appSecret string) {
	appKey, appSecret = strings.TrimSpace(appKey), strings.TrimSpace(appSecret)
	c.mu.Lock()
	if c.appKey != appKey || c.appSecret != appSecret {
		c.accessToken = ""
		c.expiresAt = time.Time{}
	}
	c.appKey = appKey
	c.appSecret = appSecret
	c.mu.Unlock()
}

// Only an authentication for the currently selected credentials may install a
// token. Detached issuances for a previous account can still populate its cache.
func (c *Client) setCredentialToken(appKey, appSecret, token string, expiresAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.appKey != appKey || c.appSecret != appSecret {
		return false
	}
	c.accessToken, c.expiresAt = token, expiresAt
	return true
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

func (c *Client) urls() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL, c.wsURL
}

func (c *Client) getMACAddress() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.macAddress
}

func (c *Client) isTokenValid() bool {
	appKey, appSecret := c.getCredentials()
	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if appKey != "" {
		if token, expiresAt, ok := tm.GetToken(appKey); ok {
			return c.setCredentialToken(appKey, appSecret, token, expiresAt)
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
	_, err := c.authenticateSelectedCredentials(ctx, appKey, appSecret)
	return err
}

// CallEndpoint executes an LS REST endpoint using a TR code and raw request body.
func (c *Client) CallEndpoint(ctx context.Context, method, path, trCD string, request any) (map[string]any, error) {
	return c.callEndpoint(ctx, method, path, trCD, request, callOptions{})
}

func (c *Client) callEndpoint(
	ctx context.Context,
	method, path, trCD string,
	request any,
	opts callOptions,
) (map[string]any, error) {
	page, err := c.callEndpointAttempt(ctx, method, path, trCD, request, opts, true)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}

func (c *Client) callEndpointAttempt(
	ctx context.Context,
	method, path, trCD string,
	request any,
	opts callOptions,
	retryUnauthorized bool,
) (*EndpointPage, error) {
	trCD = strings.TrimSpace(trCD)
	if trCD == "" {
		return nil, fmt.Errorf("%w: tr_cd is required", broker.ErrUpstreamBadRequest)
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}
	if limiter := c.trLimiter(trCD); limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}
	baseURL, _ := c.urls()
	urlPath := normalizePath(path)
	u := strings.TrimRight(baseURL, "/") + urlPath

	bodyMap, err := normalizeRequestBody(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal LS request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create LS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	token, _ := c.getToken()
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("tr_cd", trCD)
	trCont := strings.TrimSpace(opts.TRCont)
	if trCont == "" {
		trCont = "N"
	}
	req.Header.Set("tr_cont", trCont)
	req.Header.Set("tr_cont_key", strings.TrimSpace(opts.TRContKey))
	if mac := c.getMACAddress(); mac != "" {
		req.Header.Set("mac_address", mac)
	}
	for k, v := range opts.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do LS request %s %s: %w", method, urlPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read LS response: %w", err)
	}
	if resp.StatusCode >= 400 {
		err := mapHTTPError(resp.StatusCode, bodyBytes)
		if retryUnauthorized && errors.Is(err, broker.ErrUnauthorized) {
			if refreshErr := c.invalidateAndRefresh(ctx, token); refreshErr != nil {
				return nil, fmt.Errorf("token refresh after LS unauthorized response failed: %w", refreshErr)
			}
			return c.callEndpointAttempt(ctx, method, path, trCD, request, opts, false)
		}
		return nil, err
	}

	out := make(map[string]any)
	if len(bytes.TrimSpace(bodyBytes)) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
		decoder.UseNumber()
		if err := decoder.Decode(&out); err != nil {
			return nil, fmt.Errorf("decode LS response: %w", err)
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: trailing data in LS response", broker.ErrServerError)
		}
	}
	if isEmptyLSResponse(out) {
		return nil, fmt.Errorf("%w: LS empty response for tr_cd %s", broker.ErrServerError, trCD)
	}
	if code := strings.TrimSpace(asString(out["rsp_cd"])); code != "" && !isSuccessCode(code) {
		err := mapLSStatus(code, asString(out["rsp_msg"]))
		// A status in the body means LS processed the request, so an order
		// TR is never sent a second time from here.
		if retryUnauthorized && errors.Is(err, broker.ErrUnauthorized) && !isOrderTR(trCD) {
			if refreshErr := c.invalidateAndRefresh(ctx, token); refreshErr != nil {
				return nil, fmt.Errorf("token refresh after LS unauthorized response failed: %w", refreshErr)
			}
			return c.callEndpointAttempt(ctx, method, path, trCD, request, opts, false)
		}
		return nil, err
	}
	return &EndpointPage{Data: out, Continuation: responseContinuation(resp.Header)}, nil
}

func (c *Client) trLimiter(trCD string) *ratelimit.Limiter {
	rps, burst, ok := lsTRRateLimit(trCD)
	if !ok {
		return nil
	}
	appKey, _ := c.getCredentials()
	if appKey == "" {
		appKey = "default"
	}
	return ratelimit.Shared("ls-"+strings.ToLower(strings.TrimSpace(trCD)), rps, burst, appKey)
}

func lsTRRateLimit(trCD string) (float64, int, bool) {
	switch strings.ToLower(strings.TrimSpace(trCD)) {
	case "cspaq13700":
		return 1, 1, true
	case "t0424", "t0425":
		return 2, 1, true
	case "cspat00601":
		return 10, 1, true
	case "cspat00701", "cspat00801":
		return 3, 1, true
	case TRStockChart, TROverseasStockChart, TRForeignIndexHistory, TRForeignIndexQuote:
		return 1, 1, true
	case TROverseasStockQuote, "g3102", TROverseasStockInstrument, "g3106", TROverseasStockMaster:
		return 10, 1, true
	case TRStockMaster:
		return 2, 1, true
	default:
		return 0, 0, false
	}
}

func (c *Client) refreshToken(ctx context.Context) error {
	appKey, appSecret := c.getCredentials()
	if appKey == "" || appSecret == "" {
		return broker.ErrInvalidCredentials
	}
	_, err := c.authenticateSelectedCredentials(ctx, appKey, appSecret)
	return err
}

// invalidateAndRefresh drops the token LS just rejected and authenticates
// again. A token another request has already replaced is left alone.
func (c *Client) invalidateAndRefresh(ctx context.Context, rejected string) error {
	appKey, _ := c.getCredentials()
	if current, _ := c.getToken(); current == rejected {
		c.setToken("", time.Time{})
	}

	tm := c.tokenManager
	if tm == nil {
		tm = GetTokenManager()
	}
	if appKey != "" {
		_ = tokencache.InvalidateRejected(tm, appKey, rejected)
	}
	return c.refreshToken(ctx)
}

// responseContinuation reads the paging headers of an LS response. Only "Y"
// means another page exists. LS does not confine itself to Y and N: t0424
// answers tr_cont "0" on a complete response, and other TRs leave it empty,
// so every other value is read as "no more pages" instead of rejected.
func responseContinuation(h http.Header) Continuation {
	flag := strings.ToUpper(strings.TrimSpace(h.Get("tr_cont")))
	switch flag {
	case "Y":
		return Continuation{TRCont: "Y", TRContKey: strings.TrimSpace(h.Get("tr_cont_key"))}
	case "":
		return Continuation{}
	default:
		return Continuation{TRCont: "N"}
	}
}

// maxResponseBytes caps a response body held in memory. The largest LS
// payloads (symbol masters) are a few megabytes.
const maxResponseBytes = 64 << 20

// isSuccessCode reports whether rsp_cd is one of LS's "normal" message codes.
// LS numbers its messages 00000-00999 for success, 01000-07999 for business
// errors and 08000 up for system errors, so success is a range, not 00000
// alone. Observed on the live API: CSPAQ12200 answers 00136 ("조회가
// 완료되었습니다") and an empty CSPAQ13700 answers 00200 ("조회내역이
// 없습니다"). Accepted orders are reported to answer 00040 (buy) and 00039
// (sell); those two were not exercised.
func isSuccessCode(code string) bool {
	if len(code) != 5 {
		return false
	}
	n, err := strconv.Atoi(code)
	return err == nil && n >= 0 && n < 1000
}

// isOrderTR reports whether trCD places, modifies or cancels an order.
func isOrderTR(trCD string) bool {
	switch strings.ToUpper(strings.TrimSpace(trCD)) {
	case "CSPAT00601", "CSPAT00701", "CSPAT00801":
		return true
	default:
		return false
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func normalizeRequestBody(request any) (map[string]any, error) {
	switch v := request.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return maps.Clone(v), nil
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = val
		}
		return out, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		out := make(map[string]any)
		if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return out, nil
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("decode request body: %w", err)
		}
		return out, nil
	}
}

func mapHTTPError(status int, body []byte) error {
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	code := strings.TrimSpace(asString(payload["error_code"]))
	msg := strings.TrimSpace(asString(payload["error_description"]))
	if msg == "" {
		msg = strings.TrimSpace(asString(payload["rsp_msg"]))
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	if isUnauthorizedMessage(code, msg) {
		return fmt.Errorf("%w: LS %s %s", broker.ErrUnauthorized, code, msg)
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: LS %s %s", broker.ErrUnauthorized, code, msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: LS %s %s", broker.ErrRateLimitExceeded, code, msg)
	case http.StatusBadRequest:
		return fmt.Errorf("%w: LS %s %s", broker.ErrUpstreamBadRequest, code, msg)
	default:
		return fmt.Errorf("%w: LS HTTP %d %s %s", broker.ErrServerError, status, code, msg)
	}
}

func isEmptyLSResponse(out map[string]any) bool {
	if len(out) == 0 {
		return true
	}
	for key, value := range out {
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "rsp_cd", "rsp_msg":
			if strings.TrimSpace(asString(value)) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// mapLSStatus classifies a non-success rsp_cd. Gateway codes (IGW…) are
// credential or token problems; numeric codes follow LS's message ranges.
func mapLSStatus(code, msg string) error {
	code = strings.TrimSpace(code)
	msg = strings.TrimSpace(msg)
	if code == "" {
		return nil
	}
	if msg == "" {
		msg = "LS API error"
	}
	switch {
	case strings.HasPrefix(code, "IGW"):
		return fmt.Errorf("%w: LS %s %s", broker.ErrUnauthorized, code, msg)
	case strings.HasPrefix(code, "Q"):
		return fmt.Errorf("%w: LS %s %s", broker.ErrUpstreamBadRequest, code, msg)
	}
	if n, err := strconv.Atoi(code); err == nil && n >= 1000 && n < 8000 {
		// A business rejection (bad order, insufficient funds, closed
		// market): the caller's request, not a gateway fault.
		return fmt.Errorf("%w: LS %s %s", broker.ErrUpstreamBadRequest, code, msg)
	}
	return fmt.Errorf("%w: LS %s %s", broker.ErrServerError, code, msg)
}

func isUnauthorizedMessage(code, msg string) bool {
	text := strings.ToLower(strings.TrimSpace(code + " " + msg))
	return strings.Contains(text, "token") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden") ||
		strings.Contains(text, "인증") ||
		strings.Contains(text, "권한") ||
		strings.Contains(text, "토큰")
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func tokenRequestValues(appKey, appSecret string) url.Values {
	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("appkey", appKey)
	values.Set("appsecretkey", appSecret)
	values.Set("scope", "oob")
	return values
}
