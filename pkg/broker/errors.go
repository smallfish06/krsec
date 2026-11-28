package broker

import "errors"

// Sentinel errors adapters wrap so callers can classify a failure with
// errors.Is. The HTTP server maps each to a status code.
var (
	// ErrUnauthorized means the broker rejected the access token or the
	// account lacks permission for the operation.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrInvalidCredentials means the app key or secret is missing or wrong.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrTokenExpired means the access token lapsed and could not be renewed.
	ErrTokenExpired = errors.New("token expired")

	ErrInvalidSymbol = errors.New("invalid symbol")

	// ErrInvalidMarket means the market is unknown to the broker. A market
	// the broker knows but cannot serve is ErrNotSupported.
	ErrInvalidMarket = errors.New("invalid market")

	ErrInsufficientBalance = errors.New("insufficient balance")

	ErrOrderNotFound = errors.New("order not found")

	ErrInstrumentNotFound = errors.New("instrument not found")

	// ErrInvalidOrderRequest means the order was rejected before it reached
	// the broker: nothing was submitted.
	ErrInvalidOrderRequest = errors.New("invalid order request")

	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrServerError means the broker failed or answered with something
	// unusable. For an order, the outcome is unknown: check before retrying.
	ErrServerError = errors.New("server error")

	// ErrUpstreamBadRequest means the broker understood the request and
	// refused it, for example a business rule rejecting an order.
	ErrUpstreamBadRequest = errors.New("upstream bad request")

	// ErrNotSupported means the selected broker cannot perform the operation.
	ErrNotSupported = errors.New("not supported")
)
