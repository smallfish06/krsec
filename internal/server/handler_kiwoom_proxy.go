package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/endpointpath"
	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

type kiwoomProxyRequest struct {
	AccountID    string                    `json:"account_id,omitempty"`
	Method       string                    `json:"method,omitempty"`
	APIID        string                    `json:"api_id"`
	Params       map[string]any            `json:"params,omitempty"`
	Query        map[string]any            `json:"query,omitempty"`
	Body         map[string]any            `json:"body,omitempty"`
	Continuation *kiwoomspecs.Continuation `json:"continuation,omitempty"`
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

type kiwoomPageCaller interface {
	CallEndpointPage(context.Context, string, string, string, any, kiwoomspecs.Continuation) (*kiwoomspecs.EndpointPage, error)
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
	continuation, err := kiwoomRequestContinuation(c.Header("cont-yn"), c.Header("next-key"), req.Continuation)
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
	}
	result, next, err := callKiwoomProxyPage(c.Context(), impl, method, rawPath, apiID, request, continuation)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:     false,
			Error:  err.Error(),
			Broker: brk.Name(),
		})
	}
	setKiwoomContinuationHeaders(c, next)

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

		continuation, err := kiwoomRequestContinuation(c.Header("cont-yn"), c.Header("next-key"), nil)
		if err != nil {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
		}
		result, next, err := callKiwoomProxyPage(c.Context(), impl, http.MethodPost, rawPath, fixedAPIID, reqBody, continuation)
		if err != nil {
			return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
				OK:     false,
				Error:  err.Error(),
				Broker: brk.Name(),
			})
		}
		setKiwoomContinuationHeaders(c, next)

		return respond(c, http.StatusOK, Response{
			OK:     true,
			Data:   result,
			Broker: brk.Name(),
		})
	}
}

func kiwoomRequestContinuation(contYN, nextKey string, body *kiwoomspecs.Continuation) (kiwoomspecs.Continuation, error) {
	continuation := kiwoomspecs.Continuation{ContYN: strings.ToUpper(strings.TrimSpace(contYN)), NextKey: strings.TrimSpace(nextKey)}
	if body != nil {
		fromBody := kiwoomspecs.Continuation{ContYN: strings.ToUpper(strings.TrimSpace(body.ContYN)), NextKey: strings.TrimSpace(body.NextKey)}
		if (continuation.ContYN != "" || continuation.NextKey != "") && continuation != fromBody {
			return continuation, fmt.Errorf("conflicting Kiwoom continuation headers and body")
		}
		continuation = fromBody
	}
	if continuation.ContYN != "" && continuation.ContYN != "N" && continuation.ContYN != "Y" {
		return continuation, fmt.Errorf("cont-yn must be Y or N")
	}
	if (continuation.ContYN == "Y") != (continuation.NextKey != "") {
		return continuation, fmt.Errorf("cont-yn=Y and next-key must be supplied together")
	}
	return continuation, nil
}

func callKiwoomProxyPage(ctx context.Context, impl kiwoomEndpointCaller, method, path, apiID string, request any, continuation kiwoomspecs.Continuation) (any, *kiwoomspecs.Continuation, error) {
	if paged, ok := impl.(kiwoomPageCaller); ok {
		page, err := paged.CallEndpointPage(ctx, method, path, apiID, request, continuation)
		if err != nil {
			return nil, nil, err
		}
		if page == nil {
			return nil, nil, fmt.Errorf("kiwoom endpoint returned no page")
		}
		return page.Data, &page.Continuation, nil
	}
	if continuation.ContYN == "Y" || continuation.NextKey != "" {
		return nil, nil, fmt.Errorf("%w: selected Kiwoom adapter does not support continuation", broker.ErrInvalidOrderRequest)
	}
	data, err := impl.CallEndpoint(ctx, method, path, apiID, request)
	return data, nil, err
}

func setKiwoomContinuationHeaders(c interface{ SetHeader(string, string) }, continuation *kiwoomspecs.Continuation) {
	if continuation == nil {
		return
	}
	c.SetHeader("cont-yn", continuation.ContYN)
	c.SetHeader("next-key", continuation.NextKey)
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
