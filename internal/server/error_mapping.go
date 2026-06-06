package server

import (
	"errors"
	"net/http"

	"github.com/smallfish06/krsec/pkg/broker"
)

func statusFromBrokerError(err error, fallback int) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, broker.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, broker.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, broker.ErrTokenExpired):
		return http.StatusUnauthorized
	case errors.Is(err, broker.ErrInvalidSymbol):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrInvalidMarket):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrInvalidOrderRequest):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrInsufficientBalance):
		return http.StatusConflict
	case errors.Is(err, broker.ErrOrderNotFound):
		return http.StatusNotFound
	case errors.Is(err, broker.ErrInstrumentNotFound):
		return http.StatusNotFound
	case errors.Is(err, broker.ErrRateLimitExceeded):
		return http.StatusTooManyRequests
	case errors.Is(err, broker.ErrUpstreamBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrNotSupported):
		return http.StatusNotImplemented
	case errors.Is(err, broker.ErrServerError):
		return http.StatusBadGateway
	default:
		return fallback
	}
}
