package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

type orderGetter interface {
	GetOrder(ctx context.Context, orderID string) (*broker.OrderResult, error)
}

type orderFillsGetter interface {
	GetOrderFills(ctx context.Context, orderID string) ([]broker.OrderFill, error)
}

func (s *Server) handleGetOrder(c fuego.ContextNoBody) (Response, error) {
	accountID := c.PathParam("account_id")
	orderID := c.PathParam("order_id")
	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	getter, ok := brk.(orderGetter)
	if !ok {
		return respond(c, http.StatusNotImplemented, Response{
			OK:    false,
			Error: "order status lookup not supported by broker",
		})
	}

	result, err := getter.GetOrder(c.Context(), orderID)
	if err == nil {
		return respond(c, http.StatusOK, Response{
			OK:     true,
			Data:   result,
			Broker: brk.Name(),
		})
	}
	if errors.Is(err, broker.ErrOrderNotFound) {
		return respond(c, http.StatusNotFound, Response{
			OK:    false,
			Error: "order not found",
		})
	}

	return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
		OK:    false,
		Error: err.Error(),
	})
}

func (s *Server) handleGetOrderFills(c fuego.ContextNoBody) (Response, error) {
	accountID := c.PathParam("account_id")
	orderID := c.PathParam("order_id")
	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	getter, ok := brk.(orderFillsGetter)
	if !ok {
		return respond(c, http.StatusNotImplemented, Response{
			OK:    false,
			Error: "order fills lookup not supported by broker",
		})
	}

	fills, err := getter.GetOrderFills(c.Context(), orderID)
	if err == nil {
		return respond(c, http.StatusOK, Response{
			OK:     true,
			Data:   fills,
			Broker: brk.Name(),
		})
	}
	if errors.Is(err, broker.ErrOrderNotFound) {
		return respond(c, http.StatusNotFound, Response{
			OK:    false,
			Error: "order not found",
		})
	}

	return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
		OK:    false,
		Error: err.Error(),
	})
}

func (s *Server) handlePlaceOrder(c fuego.ContextWithBody[broker.OrderRequest]) (Response, error) {
	accountID := c.PathParam("account_id")

	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{
			OK:    false,
			Error: "invalid request body",
		})
	}

	if req.AccountID != "" && !sameAccountID(req.AccountID, accountID) {
		return respond(c, http.StatusBadRequest, Response{
			OK:    false,
			Error: "account_id in body does not match path",
		})
	}
	req.AccountID = accountID
	if err := normalizeOrderRequest(&req); err != nil {
		return respond(c, http.StatusBadRequest, Response{Error: err.Error()})
	}

	result, err := brk.PlaceOrder(c.Context(), req)
	if err != nil {
		status := statusFromBrokerError(err, http.StatusInternalServerError)
		return respond(c, status, Response{
			OK:    false,
			Error: err.Error(),
		})
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   result,
		Broker: brk.Name(),
	})
}

func (s *Server) handleCancelOrder(c fuego.ContextNoBody) (Response, error) {
	accountID := c.PathParam("account_id")
	orderID := c.PathParam("order_id")
	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	err := brk.CancelOrder(c.Context(), orderID)
	if err == nil {
		return respond(c, http.StatusOK, Response{
			OK:     true,
			Broker: brk.Name(),
		})
	}

	if errors.Is(err, broker.ErrOrderNotFound) {
		return respond(c, http.StatusNotFound, Response{
			OK:    false,
			Error: "order not found",
		})
	}

	return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
		OK:    false,
		Error: err.Error(),
	})
}

func (s *Server) handleModifyOrder(c fuego.ContextWithBody[broker.ModifyOrderRequest]) (Response, error) {
	accountID := c.PathParam("account_id")
	orderID := c.PathParam("order_id")
	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{OK: false, Error: reason})
	}

	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{
			OK:    false,
			Error: "invalid request body",
		})
	}

	req.Type = broker.OrderType(strings.ToLower(strings.TrimSpace(string(req.Type))))
	if req.Type != "" && req.Type != broker.OrderTypeLimit && req.Type != broker.OrderTypeMarket {
		return respond(c, http.StatusBadRequest, Response{Error: "type must be limit or market"})
	}

	result, err := brk.ModifyOrder(c.Context(), orderID, req)
	if err == nil {
		return respond(c, http.StatusOK, Response{
			OK:     true,
			Data:   result,
			Broker: brk.Name(),
		})
	}

	if errors.Is(err, broker.ErrOrderNotFound) {
		return respond(c, http.StatusNotFound, Response{
			OK:    false,
			Error: "order not found",
		})
	}

	return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
		OK:    false,
		Error: err.Error(),
	})
}

// normalizeOrderRequest canonicalizes side and type ("BUY" and " buy " are the
// same order) and rejects values no broker understands. The check lives at the
// HTTP edge because the brokers' own APIs cannot be relied on to refuse them.
func normalizeOrderRequest(req *broker.OrderRequest) error {
	req.Side = broker.OrderSide(strings.ToLower(strings.TrimSpace(string(req.Side))))
	if req.Side != broker.OrderSideBuy && req.Side != broker.OrderSideSell {
		return errors.New("side must be buy or sell")
	}
	req.Type = broker.OrderType(strings.ToLower(strings.TrimSpace(string(req.Type))))
	if req.Type != broker.OrderTypeLimit && req.Type != broker.OrderTypeMarket {
		return errors.New("type must be limit or market")
	}
	req.Symbol = strings.TrimSpace(req.Symbol)
	if req.Symbol == "" {
		return errors.New("symbol is required")
	}
	return nil
}
