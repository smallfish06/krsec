package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

type instrumentGetter interface {
	GetInstrument(ctx context.Context, market, symbol string) (*broker.Instrument, error)
}

func (s *Server) handleGetInstrument(c fuego.ContextNoBody) (Response, error) {
	market := c.PathParam("market")
	symbol := c.PathParam("symbol")
	accountID := c.QueryParam("account_id")

	var candidates []broker.Broker
	if accountID != "" {
		brk, status, reason := s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return respond(c, status, Response{OK: false, Error: reason})
		}
		candidates = []broker.Broker{brk}
	} else {
		candidates = s.orderedBrokers()
	}

	if len(candidates) == 0 {
		return respond(c, http.StatusServiceUnavailable, Response{OK: false, Error: "no broker available"})
	}

	var firstErr error
	supported := false
	for _, brk := range candidates {
		getter, ok := brk.(instrumentGetter)
		if !ok {
			continue
		}
		supported = true

		result, err := getter.GetInstrument(c.Context(), market, symbol)
		if err == nil {
			return respond(c, http.StatusOK, Response{
				OK:     true,
				Data:   result,
				Broker: brk.Name(),
			})
		}

		if errors.Is(err, broker.ErrInstrumentNotFound) {
			continue
		}
		if errors.Is(err, broker.ErrInvalidMarket) || errors.Is(err, broker.ErrInvalidSymbol) {
			return respond(c, http.StatusBadRequest, Response{OK: false, Error: err.Error()})
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return respond(c, http.StatusInternalServerError, Response{OK: false, Error: firstErr.Error()})
	}
	if !supported {
		return respond(c, http.StatusNotImplemented, Response{OK: false, Error: "instrument lookup not supported by broker"})
	}

	return respond(c, http.StatusNotFound, Response{OK: false, Error: "instrument not found"})
}
