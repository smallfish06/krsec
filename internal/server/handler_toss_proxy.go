package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/toss"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	tossspecs "github.com/smallfish06/krsec/pkg/toss/specs"
)

type tossProxyRequest struct {
	AccountID string                 `json:"account_id,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Query     map[string]interface{} `json:"query,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
	Body      map[string]interface{} `json:"body,omitempty"`
}

type tossEndpointCaller interface {
	CallEndpoint(ctx context.Context, method, path string, query map[string]interface{}, body interface{}) (interface{}, error)
}

// handleTossProxy handles POST /toss/{path...}.
func (s *Server) handleTossProxy(c fuego.ContextWithBody[tossProxyRequest]) (Response, error) {
	rawPath := normalizeTossProxyPath(c.PathParam("path"))
	if rawPath == "" {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "path is required"})
	}
	if rawPath == toss.PathOAuthToken {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "use /auth/token for Toss OAuth token issuance"})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "invalid request body"})
	}
	if err := validateTossProxyRequest(&req); err != nil {
		s.logger.Warn("Toss proxy validation failed", "path", rawPath, "account_id", req.AccountID, "error", err)
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}

	method := req.Method
	if method == "" {
		var ok bool
		method, ok = tossspecs.DefaultMethodForPath(rawPath)
		if !ok {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: "method is required for ambiguous or unsupported Toss path"})
		}
	}
	spec, ok := tossspecs.LookupDocumentedEndpointSpec(method, rawPath)
	if !ok {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "unsupported Toss endpoint"})
	}

	brk, status, reason := s.resolveTossProxyBroker(req.AccountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}
	if spec.AccountRequired {
		if _, ok := s.resolveTossProxyAccountConfig(req.AccountID); !ok {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected Toss account is missing account_seq"})
		}
	}

	impl, ok := brk.(tossEndpointCaller)
	if !ok {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support Toss endpoint dispatch"})
	}

	query := mergeInterfaceMaps(req.Query, req.Params)
	result, err := impl.CallEndpoint(c.Context(), method, rawPath, query, req.Body)
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

func (s *Server) resolveTossProxyBroker(accountID string) (broker.Broker, int, string) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		brk, status, reason := s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return nil, status, reason
		}
		if !strings.EqualFold(strings.TrimSpace(brk.Name()), broker.NameToss) {
			return nil, http.StatusBadRequest, "account broker is not Toss"
		}
		return brk, 0, ""
	}

	for _, acc := range s.accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Broker), broker.CodeToss) {
			continue
		}
		if brk, ok := s.getBrokerStrict(acc.AccountID); ok {
			return brk, 0, ""
		}
	}
	return nil, http.StatusServiceUnavailable, "no Toss account available"
}

func (s *Server) resolveTossProxyAccountConfig(accountID string) (configAccount config.AccountConfig, ok bool) {
	accountID = strings.TrimSpace(accountID)
	for _, acc := range s.accounts {
		if !strings.EqualFold(strings.TrimSpace(acc.Broker), broker.CodeToss) {
			continue
		}
		if accountID != "" && !sameAccountID(acc.AccountID, accountID) {
			continue
		}
		if strings.TrimSpace(acc.AccountSeq) == "" {
			return config.AccountConfig{}, false
		}
		return acc, true
	}
	return config.AccountConfig{}, false
}

func normalizeTossProxyPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
