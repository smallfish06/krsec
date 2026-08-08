// Package kiwoom exposes the public Kiwoom Securities adapter.
package kiwoom

import (
	"context"

	internaladapter "github.com/smallfish06/krsec/internal/kiwoom/adapter"
	"github.com/smallfish06/krsec/pkg/adapter"
)

// Adapter is the public Kiwoom adapter contract.
type Adapter interface {
	adapter.Adapter
	CallEndpoint(ctx context.Context, method, path, apiID string, request any) (any, error)
}

// NewAdapterWithOptions creates a Kiwoom adapter with injectable options.
func NewAdapterWithOptions(sandbox bool, accountID string, opts adapter.Options) Adapter {
	return internaladapter.NewAdapterWithOptions(sandbox, accountID, opts.TokenManager, opts.OrderContextDir, opts.Logger)
}
