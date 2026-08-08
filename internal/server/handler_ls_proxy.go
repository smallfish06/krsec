package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

type lsProxyRequest struct {
	AccountID string         `json:"account_id,omitempty"`
	Method    string         `json:"method,omitempty"`
	TRCD      string         `json:"tr_cd"`
	Params    map[string]any `json:"params,omitempty"`
	Query     map[string]any `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

type lsEndpointCaller interface {
	CallEndpoint(ctx context.Context, method, path, trCD string, request any) (any, error)
}

// handleLSProxy handles POST /ls/{path...}.
func (s *Server) handleLSProxy(c fuego.ContextWithBody[lsProxyRequest]) (Response, error) {
	rawPath := normalizeLSProxyPath(c.PathParam("path"))
	if rawPath == "" {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "path is required"})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "invalid request body"})
	}
	if err := validateLSProxyRequest(&req); err != nil {
		s.logger.Warn("LS proxy validation failed", "path", rawPath, "account_id", req.AccountID, "error", err)
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}

	method := req.Method
	if method == "" {
		method = http.MethodPost
	}

	brk, status, reason := s.resolveLSProxyBroker(req.AccountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}
	impl, ok := brk.(lsEndpointCaller)
	if !ok {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support LS endpoint dispatch"})
	}

	request := mergeInterfaceMaps(
		mergeInterfaceMaps(req.Query, req.Params),
		req.Body,
	)
	result, err := impl.CallEndpoint(c.Context(), method, rawPath, req.TRCD, request)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:     false,
			Error:  err.Error(),
			Broker: brk.Name(),
		})
	}
	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   result,
		Broker: brk.Name(),
	})
}

func (s *Server) resolveLSProxyBroker(accountID string) (broker.Broker, int, string) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		brk, status, reason := s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return nil, status, reason
		}
		if !strings.EqualFold(strings.TrimSpace(brk.Name()), broker.NameLS) {
			return nil, http.StatusBadRequest, "account broker is not LS"
		}
		return brk, 0, ""
	}

	for _, acc := range s.accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Broker), broker.CodeLS) {
			continue
		}
		if brk, ok := s.getBrokerStrict(acc.AccountID); ok {
			return brk, 0, ""
		}
	}
	return nil, http.StatusServiceUnavailable, "no LS account available"
}

func normalizeLSProxyPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
