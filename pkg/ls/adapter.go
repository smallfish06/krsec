package ls

import (
	"context"

	internal "github.com/smallfish06/krsec/internal/ls"
	internaladapter "github.com/smallfish06/krsec/internal/ls/adapter"
	"github.com/smallfish06/krsec/pkg/adapter"
)

// Adapter is the public LS adapter contract.
type Adapter interface {
	adapter.Adapter
	CallEndpoint(ctx context.Context, method, path, trCD string, request any) (any, error)
	ConnectRealtime(ctx context.Context) (*RealtimeConn, error)
	BuildTradeSubscriptions(ctx context.Context) ([]RealtimeSubscription, error)
	BuildOverseasTradeSubscriptions(ctx context.Context, market string, maxRows int) ([]RealtimeSubscription, error)
}

type RealtimeConn = internal.RealtimeConn
type RealtimeMessage = internal.RealtimeMessage
type RealtimeSubscription = internal.RealtimeSubscription

// OverseasRealtimeKey returns the fixed-width realtime key used by LS GSC/GSH.
func OverseasRealtimeKey(exchange, symbol string) (string, error) {
	return internal.OverseasRealtimeKey(exchange, symbol)
}

// Options configures LS adapter internals.
type Options struct {
	adapter.Options
	MACAddress string
}

// NewAdapterWithOptions creates an LS adapter with injectable options.
func NewAdapterWithOptions(sandbox bool, accountID string, opts Options) Adapter {
	return internaladapter.NewAdapterWithOptions(sandbox, accountID, opts.TokenManager, opts.MACAddress, opts.Logger)
}
