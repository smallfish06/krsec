package toss

import (
	"context"

	internaladapter "github.com/smallfish06/krsec/internal/toss/adapter"
	"github.com/smallfish06/krsec/pkg/adapter"
)

// Adapter is the public Toss adapter contract.
type Adapter interface {
	adapter.Adapter
	CallEndpoint(ctx context.Context, method, path string, query map[string]any, body any) (any, error)
}

// NewAdapterWithOptions creates a Toss adapter with injectable options.
func NewAdapterWithOptions(sandbox bool, accountID, accountSeq string, opts adapter.Options) Adapter {
	return internaladapter.NewAdapterWithOptions(sandbox, accountID, accountSeq, opts.TokenManager, opts.Logger)
}
