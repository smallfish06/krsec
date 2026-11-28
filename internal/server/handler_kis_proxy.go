package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/endpointpath"
	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/pkg/broker"
)

// cacheStatusHeader tells a KIS proxy caller whether the payload came from
// the upstream ("miss"), the cache ("hit"), or an expired entry served because
// the upstream call failed ("stale").
const cacheStatusHeader = "X-Krsec-Cache"

type kisProxyRequest struct {
	AccountID string         `json:"account_id,omitempty"`
	Method    string         `json:"method,omitempty"`
	TRID      string         `json:"tr_id"`
	Params    map[string]any `json:"params,omitempty"`
	Query     map[string]any `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

func (r *kisProxyRequest) UnmarshalJSON(data []byte) error {
	type alias kisProxyRequest
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var flat map[string]any
	if err := json.Unmarshal(data, &flat); err != nil {
		return err
	}
	delete(flat, "account_id")
	delete(flat, "method")
	delete(flat, "tr_id")
	delete(flat, "params")
	delete(flat, "query")
	delete(flat, "body")

	decoded.Params = mergeInterfaceMaps(flat, decoded.Params)

	*r = kisProxyRequest(decoded)
	return nil
}

type kisEndpointCaller interface {
	CallEndpoint(
		ctx context.Context,
		method string,
		path string,
		trID string,
		request any,
	) (any, error)
}

func (s *Server) handleKISProxyStatic(path string) func(fuego.ContextWithBody[kisProxyRequest]) (Response, error) {
	rawPath := normalizeKISProxyPath(path)
	return func(c fuego.ContextWithBody[kisProxyRequest]) (Response, error) {
		return s.handleKISProxyPath(c, rawPath)
	}
}

func (s *Server) handleKISProxyPath(c fuego.ContextWithBody[kisProxyRequest], rawPath string) (Response, error) {
	if rawPath == "" {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "path is required"})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "invalid request body"})
	}
	if err := validateKISProxyRequest(&req); err != nil {
		s.logger.Warn("KIS proxy validation failed", "path", rawPath, "account_id", req.AccountID, "error", err)
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}

	req.AccountID, err = proxyAccountSelector(req.AccountID, c.QueryParam("account_id"))
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}

	trID := req.TRID
	method := req.Method

	brk, resolvedAccountID, status, reason := s.resolveKISProxyBroker(req.AccountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	impl, ok := brk.(kisEndpointCaller)
	if !ok {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support KIS endpoint dispatch"})
	}

	request := mergeStringMaps(
		mergeStringMaps(toStringMap(req.Query), toStringMap(req.Params)),
		toStringMap(req.Body),
	)

	cacheReq := KISProxyCacheRequest{
		Method:    method,
		Path:      rawPath,
		TRID:      trID,
		AccountID: resolvedAccountID,
		Params:    maps.Clone(request),
	}
	cacheTTL, cacheable := s.kisCachePolicy(cacheReq)
	if cacheTTL <= 0 {
		cacheable = false
	}
	if !cacheable {
		result, err := s.callKISEndpoint(c.Context(), impl, method, rawPath, trID, resolvedAccountID, request)
		if err != nil {
			return s.kisProxyError(c, brk, method, rawPath, trID, err)
		}
		return respond(c, http.StatusOK, Response{OK: true, Data: result, Broker: brk.Name()})
	}

	cacheKey := buildKISProxyCacheKey(resolvedAccountID, method, rawPath, trID, request)
	if cached, ok := s.kisCache.getFresh(cacheKey); ok {
		s.logger.Debug("KIS proxy cache hit", "path", rawPath, "tr_id", trID, "account_id", resolvedAccountID)
		c.SetHeader(cacheStatusHeader, "hit")
		return respond(c, http.StatusOK, Response{OK: true, Data: cached, Broker: brk.Name()})
	}

	// Storing inside the flight keeps the result even when the request that
	// started it has gone away before the upstream answered.
	result, err := s.kisCache.do(c.Context(), cacheKey, func(ctx context.Context) (any, error) {
		value, err := s.callKISEndpoint(ctx, impl, method, rawPath, trID, resolvedAccountID, request)
		if err == nil {
			s.kisCache.set(cacheKey, value, cacheTTL)
		}
		return value, err
	})
	if err != nil {
		if stale, ok := s.kisCache.getStale(cacheKey); ok {
			s.logger.Warn("KIS proxy serving stale cache after endpoint error",
				"path", rawPath,
				"method", method,
				"tr_id", trID,
				"account_id", resolvedAccountID,
				"error", err,
			)
			c.SetHeader(cacheStatusHeader, "stale")
			return respond(c, http.StatusOK, Response{OK: true, Data: stale, Broker: brk.Name()})
		}
		return s.kisProxyError(c, brk, method, rawPath, trID, err)
	}

	c.SetHeader(cacheStatusHeader, "miss")
	return respond(c, http.StatusOK, Response{OK: true, Data: result, Broker: brk.Name()})
}

// callKISEndpoint waits for the outbound rate limit, then calls KIS.
func (s *Server) callKISEndpoint(
	ctx context.Context,
	impl kisEndpointCaller,
	method, rawPath, trID, accountID string,
	request map[string]string,
) (any, error) {
	waited, err := s.kisRateLimiter.wait(ctx, rawPath, trID)
	if err != nil {
		return nil, fmt.Errorf("kis proxy rate limit wait: %w", err)
	}
	if waited > 0 {
		s.logger.Debug("KIS proxy rate limit wait",
			"path", rawPath,
			"tr_id", trID,
			"account_id", accountID,
			"wait_ms", waited.Milliseconds(),
		)
	}
	return impl.CallEndpoint(ctx, method, rawPath, trID, request)
}

func (s *Server) kisProxyError(
	c interface{ SetStatus(int) },
	brk broker.Broker,
	method, rawPath, trID string,
	err error,
) (Response, error) {
	status := statusFromBrokerError(err, http.StatusInternalServerError)
	// A rejected request is the caller's problem, not an operational fault.
	level := slog.LevelError
	if status < http.StatusInternalServerError {
		level = slog.LevelWarn
	}
	s.logger.Log(context.Background(), level, "KIS proxy endpoint error",
		"path", rawPath,
		"method", method,
		"tr_id", trID,
		"status", status,
		"error", err,
	)
	return respond(c, status, Response{Error: err.Error(), Broker: brk.Name()})
}

func (s *Server) resolveKISProxyBroker(accountID string) (broker.Broker, string, int, string) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		brk, status, reason := s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return nil, "", status, reason
		}
		if !strings.EqualFold(strings.TrimSpace(brk.Name()), broker.NameKIS) {
			return nil, "", http.StatusBadRequest, "account broker is not KIS"
		}
		return brk, s.resolveKISProxyAccountID(accountID), 0, ""
	}

	for _, acc := range s.accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Broker), broker.CodeKIS) {
			continue
		}
		if brk, ok := s.getBrokerStrict(acc.AccountID); ok {
			return brk, strings.TrimSpace(acc.AccountID), 0, ""
		}
	}
	return nil, "", http.StatusServiceUnavailable, "no KIS account available"
}

func (s *Server) resolveKISProxyAccountID(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	if _, ok := s.brokers[accountID]; ok {
		return accountID
	}
	candidates := s.findBrokerAccountCandidates(accountID)
	if len(candidates) == 1 {
		return candidates[0]
	}
	return accountID
}

func normalizeKISProxyPath(path string) string {
	return endpointpath.Normalize(path, kis.PathPrefixUAPI, kis.PathPrefixUAPISlash)
}

func toStringMap(src map[string]any) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if v == nil {
			out[key] = ""
			continue
		}
		out[key] = fmt.Sprint(v)
	}
	return out
}

func mergeStringMaps(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := maps.Clone(base)
	if out == nil {
		out = make(map[string]string, len(override))
	}
	maps.Copy(out, override)
	return out
}

func mergeInterfaceMaps(base map[string]any, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := maps.Clone(base)
	if out == nil {
		out = make(map[string]any, len(override))
	}
	maps.Copy(out, override)
	return out
}
