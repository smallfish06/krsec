package broker

import "errors"

var (
	// ErrUnauthorized indicates authentication failure
	ErrUnauthorized = errors.New("unauthorized")

	// ErrInvalidCredentials indicates invalid credentials
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrTokenExpired indicates the token has expired
	ErrTokenExpired = errors.New("token expired")

	// ErrInvalidSymbol indicates an invalid symbol
	ErrInvalidSymbol = errors.New("invalid symbol")

	// ErrInvalidMarket indicates an invalid market
	ErrInvalidMarket = errors.New("invalid market")

	// ErrInsufficientBalance indicates insufficient balance
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrOrderNotFound indicates order not found
	ErrOrderNotFound = errors.New("order not found")

	// ErrInstrumentNotFound indicates instrument not found
	ErrInstrumentNotFound = errors.New("instrument not found")

	// ErrInvalidOrderRequest indicates an invalid order request
	ErrInvalidOrderRequest = errors.New("invalid order request")

	// ErrRateLimitExceeded indicates rate limit exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrServerError indicates a server error
	ErrServerError = errors.New("server error")

	// ErrUpstreamBadRequest indicates the upstream API rejected the request
	ErrUpstreamBadRequest = errors.New("upstream bad request")
)
