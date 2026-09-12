package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/ls"
)

type lsProxyRequest struct {
	AccountID string         `json:"account_id,omitempty"`
	Method    string         `json:"method,omitempty"`
	TRCD      string         `json:"tr_cd"`
	Params    map[string]any `json:"params,omitempty"`
	Query     map[string]any `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
	TRCont    string         `json:"tr_cont,omitempty"`
	TRContKey string         `json:"tr_cont_key,omitempty"`
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
	continuation, err := lsRequestContinuation(c.Header("tr_cont"), c.Header("tr_cont_key"), req.TRCont, req.TRContKey)
	if err != nil {
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
	var result any
	if pageCaller, ok := brk.(ls.PageCaller); ok {
		var page *ls.EndpointPage
		page, err = pageCaller.CallEndpointPage(c.Context(), method, rawPath, req.TRCD, request, continuation)
		if err == nil && page == nil {
			err = fmt.Errorf("%w: LS endpoint page missing", broker.ErrServerError)
		}
		if err == nil && page != nil {
			result = page.Data
			c.SetHeader("tr_cont", page.TRCont)
			c.SetHeader("tr_cont_key", page.TRContKey)
		}
	} else if continuation.TRCont != "" || continuation.TRContKey != "" {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: "selected account does not support LS continuation"})
	} else {
		result, err = impl.CallEndpoint(c.Context(), method, rawPath, req.TRCD, request)
	}
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

func lsRequestContinuation(headerCont, headerKey, bodyCont, bodyKey string) (ls.Continuation, error) {
	headerCont = strings.ToUpper(strings.TrimSpace(headerCont))
	bodyCont = strings.ToUpper(strings.TrimSpace(bodyCont))
	headerKey = strings.TrimSpace(headerKey)
	bodyKey = strings.TrimSpace(bodyKey)
	if (headerCont != "" && bodyCont != "" && headerCont != bodyCont) || (headerKey != "" && bodyKey != "" && headerKey != bodyKey) {
		return ls.Continuation{}, fmt.Errorf("conflicting LS continuation headers and body")
	}
	if bodyCont == "" {
		bodyCont = headerCont
	}
	if bodyKey == "" {
		bodyKey = headerKey
	}
	if (bodyCont != "" && bodyCont != "Y" && bodyCont != "N") || (bodyCont == "Y" && bodyKey == "") || (bodyKey != "" && bodyCont != "Y") {
		return ls.Continuation{}, fmt.Errorf("continuation requires tr_cont=Y and a non-empty tr_cont_key")
	}
	return ls.Continuation{TRCont: bodyCont, TRContKey: bodyKey}, nil
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
