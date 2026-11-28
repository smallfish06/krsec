package broker

import "context"

// Broker is what every broker adapter implements and what the HTTP server
// dispatches to. An adapter is bound to one account; accountID parameters
// exist so a caller holding several adapters can assert which one it meant.
//
// Methods return the sentinel errors in this package (wrapped with detail)
// so callers can branch with errors.Is. An operation a broker cannot perform
// returns ErrNotSupported instead of an approximation.
type Broker interface {
	// Name returns the broker's display name, one of the Name* constants for
	// built-in adapters.
	Name() string

	// Authenticate issues an access token, or returns the cached one while it
	// is still valid. Adapters also call it themselves when a token expires.
	Authenticate(ctx context.Context, creds Credentials) (*Token, error)

	// GetQuote returns the latest price. market selects the venue ("KRX",
	// "NXT", "US-NASDAQ", ...); the accepted values differ per broker.
	GetQuote(ctx context.Context, market, symbol string) (*Quote, error)

	// GetOHLCV returns candles oldest first, bounded by opts.
	GetOHLCV(ctx context.Context, market, symbol string, opts OHLCVOpts) ([]OHLCV, error)

	// GetBalance returns cash and valuation totals. Metrics the broker does
	// not report are named in Balance.UnavailableFields rather than zeroed.
	GetBalance(ctx context.Context, accountID string) (*Balance, error)

	GetPositions(ctx context.Context, accountID string) ([]Position, error)

	// PlaceOrder submits an order and returns once the broker has accepted
	// it; the result is normally still pending.
	PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error)

	// CancelOrder cancels whatever quantity of the order is still open.
	CancelOrder(ctx context.Context, orderID string) error

	// ModifyOrder changes the price or quantity of an open order. Some
	// brokers replace the order: use the OrderID in the result afterwards.
	ModifyOrder(ctx context.Context, orderID string, req ModifyOrderRequest) (*OrderResult, error)
}
