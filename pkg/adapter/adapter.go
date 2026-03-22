package adapter

import (
	"context"
	"log/slog"

	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// Options configures broker adapter internals.
type Options struct {
	TokenManager    tokencache.Manager
	OrderContextDir string
	Logger          *slog.Logger
}

// Adapter captures the common broker adapter surface.
type Adapter interface {
	broker.Broker
	GetOrder(ctx context.Context, orderID string) (*broker.OrderResult, error)
	GetOrderFills(ctx context.Context, orderID string) ([]broker.OrderFill, error)
	GetInstrument(ctx context.Context, market, symbol string) (*broker.Instrument, error)
}
