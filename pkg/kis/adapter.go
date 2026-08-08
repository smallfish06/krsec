package kis

import (
	"context"

	internaladapter "github.com/smallfish06/krsec/internal/kis/adapter"
	"github.com/smallfish06/krsec/pkg/adapter"
)

// Adapter is the public KIS adapter contract.
type Adapter interface {
	adapter.Adapter
	CallEndpoint(ctx context.Context, method, path, trID string, request any) (any, error)
	BootstrapSymbols(ctx context.Context) (int, error)
	ReloadSymbols(ctx context.Context) (int, error)
}

// NewAdapterWithOptions creates a KIS adapter with injectable options.
func NewAdapterWithOptions(sandbox bool, accountID string, opts adapter.Options) Adapter {
	return internaladapter.NewAdapterWithOptions(sandbox, accountID, opts.TokenManager, opts.OrderContextDir, opts.Logger)
}
