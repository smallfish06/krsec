package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/endpointpath"
	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
)

type kiwoomProxyRequest struct {
	AccountID string         `json:"account_id,omitempty"`
	Method    string         `json:"method,omitempty"`
	APIID     string         `json:"api_id"`
	Params    map[string]any `json:"params,omitempty"`
	Query     map[string]any `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

type kiwoomEndpointCaller interface {
	CallEndpoint(
		ctx context.Context,
		method string,
		path string,
		apiID string,
		request any,
	) (any, error)
}

// handleKiwoomProxy handles POST /kiwoom/{path...}
func (s *Server) handleKiwoomProxy(c fuego.ContextWithBody[kiwoomProxyRequest]) (Response, error) {
	rawPath := normalizeKiwoomProxyPath(c.PathParam("path"))
	if rawPath == "" {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "path is required"})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "invalid request body"})
	}
	if err := validateKiwoomProxyRequest(&req); err != nil {
		s.logger.Warn("Kiwoom proxy validation failed", "path", rawPath, "account_id", req.AccountID, "error", err)
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}

	apiID := req.APIID
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}

	brk, status, reason := s.resolveKiwoomProxyBroker(req.AccountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	impl, ok := brk.(kiwoomEndpointCaller)
	if !ok {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support Kiwoom endpoint dispatch"})
	}

	request := mergeInterfaceMaps(
		mergeInterfaceMaps(req.Query, req.Params),
		req.Body,
	)
	result, err := impl.CallEndpoint(c.Context(), method, rawPath, apiID, request)
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

func (s *Server) handleKiwoomProxyStatic(path, apiID string) func(fuego.ContextWithBody[map[string]any]) (Response, error) {
	rawPath := normalizeKiwoomProxyPath(path)
	fixedAPIID := strings.ToLower(strings.TrimSpace(apiID))
	return func(c fuego.ContextWithBody[map[string]any]) (Response, error) {
		if rawPath == "" || fixedAPIID == "" {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: "path/api_id is required"})
		}

		reqBody, err := c.Body()
		if err != nil {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: "invalid request body"})
		}

		brk, status, reason := s.resolveKiwoomProxyBroker(c.QueryParam("account_id"))
		if brk == nil {
			return respond(c, status, Response{OK: false, Error: reason})
		}

		impl, ok := brk.(kiwoomEndpointCaller)
		if !ok {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support Kiwoom endpoint dispatch"})
		}

		result, err := impl.CallEndpoint(c.Context(), http.MethodPost, rawPath, fixedAPIID, reqBody)
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
}

func (s *Server) resolveKiwoomProxyBroker(accountID string) (broker.Broker, int, string) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		brk, status, reason := s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return nil, status, reason
		}
		if !strings.EqualFold(strings.TrimSpace(brk.Name()), broker.NameKiwoom) {
			return nil, http.StatusBadRequest, "account broker is not Kiwoom"
		}
		return brk, 0, ""
	}

	for _, acc := range s.accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Broker), broker.CodeKiwoom) {
			continue
		}
		if brk, ok := s.getBrokerStrict(acc.AccountID); ok {
			return brk, 0, ""
		}
	}
	return nil, http.StatusServiceUnavailable, "no Kiwoom account available"
}

func normalizeKiwoomProxyPath(path string) string {
	return endpointpath.Normalize(path, kiwoom.PathPrefixAPI, kiwoom.PathPrefixAPISlash)
}
