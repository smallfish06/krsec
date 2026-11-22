package adapter

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/smallfish06/krsec/internal/kis"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

func TestNewEndpointDispatcher_IncludesRequiredKISRoutes(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})

	required := []string{
		kis.PathDomesticStockInquireDailyItemChartPrice,
		kis.PathDomesticStockTradingInquirePsblRvseCncl,
		kis.PathDomesticStockDividend,
		kis.PathDomesticBondInquirePrice,
		"/uapi/domestic-bond/v1/quotations/inquire-daily-price",
		"/uapi/domestic-bond/v1/quotations/search-bond-info",
		"/uapi/domestic-bond/v1/quotations/avg-unit",
		kis.PathDomesticBondInquireBalance,
	}

	for _, path := range required {
		route, ok := d.routes[path]
		if !ok {
			t.Fatalf("missing route: %s", path)
		}
		if !route.allows(http.MethodGet) {
			t.Fatalf("route does not allow GET: %s", path)
		}
	}
}

func TestNewEndpointDispatcher_CoversAllDocumentedKISPaths(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})

	if got, want := len(d.routes), len(kisspecs.DocumentedKISEndpointSpecs); got != want {
		t.Fatalf("unexpected route count: got=%d want=%d", got, want)
	}
}

func TestEndpointDispatcher_RejectsNonUAPIUnknownPath(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})

	_, err := d.callEndpoint(context.Background(), http.MethodGet, "", "", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported KIS endpoint path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEndpointDispatcher_DocumentedUAPIValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})

	_, err := d.callEndpoint(context.Background(), http.MethodGet, "/uapi/domestic-stock/v1/quotations/chk-holiday", "", map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEndpointDispatcher_MethodMustMatchDocumentedSpec(t *testing.T) {
	t.Parallel()

	d := newEndpointDispatcher(&Adapter{})

	_, err := d.callEndpoint(context.Background(), http.MethodGet, "/uapi/domestic-stock/v1/trading/order-cash", "", map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("unexpected error: %v", err)
	}
}
