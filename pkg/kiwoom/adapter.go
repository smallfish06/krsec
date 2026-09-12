// Package kiwoom exposes the public Kiwoom Securities adapter.
package kiwoom

import (
	"context"

	internaladapter "github.com/smallfish06/krsec/internal/kiwoom/adapter"
	"github.com/smallfish06/krsec/pkg/adapter"
	"github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

// Adapter is the public Kiwoom adapter contract.
type Adapter interface {
	adapter.Adapter
	CallEndpoint(ctx context.Context, method, path, apiID string, request any) (any, error)
}

// Continuation carries Kiwoom's cont-yn and next-key headers.
type Continuation = specs.Continuation

// EndpointPage returns endpoint data together with continuation headers.
type EndpointPage = specs.EndpointPage

// PageCaller is an optional extension implemented by the built-in adapter.
// Existing custom implementations of Adapter remain source-compatible.
type PageCaller interface {
	CallEndpointPage(ctx context.Context, method, path, apiID string, request any, continuation Continuation) (*EndpointPage, error)
}

// NewAdapterWithOptions creates a Kiwoom adapter with injectable options.
func NewAdapterWithOptions(sandbox bool, accountID string, opts adapter.Options) Adapter {
	return internaladapter.NewAdapterWithOptions(sandbox, accountID, opts.TokenManager, opts.OrderContextDir, opts.Logger)
}
