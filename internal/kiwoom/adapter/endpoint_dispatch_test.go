package adapter

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

func TestNewEndpointDispatcher_CoversAllDocumentedKiwoomPathAPIID(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})
	for _, spec := range kiwoomspecs.DocumentedKiwoomEndpointSpecs {
		key := endpointRouteKey{
			path:  normalizeEndpointPath(spec.Path),
			apiID: normalizeEndpointAPIID(spec.APIID),
		}
		if key.path == "" || key.apiID == "" {
			continue
		}
		if _, ok := d.routes[key]; !ok {
			t.Fatalf("missing route for %s/%s", key.path, key.apiID)
		}
	}
}

func TestCallEndpoint_RequiresAPIID(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodPost, kiwoom.PathStockInfo, "", map[string]any{"stk_cd": "005930"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_UnsupportedPath(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodPost, "/api/unknown/path", "ka10001", map[string]any{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_UnsupportedPathMethodValidation(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodGet, "/api/unknown/path", "zz99999", map[string]any{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_UnsupportedAPIIDOnKnownPath(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodPost, kiwoom.PathStockInfo, "zz99999", map[string]any{"stk_cd": "005930"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_DocumentedRouteRequiresClient(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodPost, kiwoom.PathStockInfo, "ka10002", map[string]any{"stk_cd": "005930"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_DocumentedRouteMissingRequiredField(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), "", kiwoom.PathStockInfo, "ka10002", map[string]any{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_MethodValidation(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), http.MethodGet, kiwoom.PathStockInfo, "ka10001", map[string]any{"stk_cd": "005930"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("expected ErrInvalidOrderRequest, got %v", err)
	}
}

func TestCallEndpoint_ValidRouteMissingSymbol(t *testing.T) {
	t.Parallel()
	a := &Adapter{}

	_, err := a.CallEndpoint(context.Background(), "", kiwoom.PathStockInfo, "ka10001", map[string]any{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, broker.ErrInvalidSymbol) {
		t.Fatalf("expected ErrInvalidSymbol, got %v", err)
	}
}

func TestApplyDocumentedCustomDefaults_TicScope(t *testing.T) {
	t.Parallel()

	payload := map[string]any{}
	applyDocumentedDefaults("ka50079", payload)
	if payload["tic_scope"] != "1" {
		t.Fatalf("tic_scope = %#v, want \"1\"", payload["tic_scope"])
	}

	payload2 := map[string]any{"tic_scope": "5"}
	applyDocumentedDefaults("ka50080", payload2)
	if payload2["tic_scope"] != "5" {
		t.Fatalf("tic_scope = %#v, want \"5\"", payload2["tic_scope"])
	}
}
