package adapter

import (
	"errors"
	"regexp"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
)

// A quantity-only modify of a resting limit order used to be sent with
// orderType MARKET, which executes at market.
func TestModifyNeverGuessesMarket(t *testing.T) {
	t.Parallel()

	if _, err := buildModifyOrderBody(broker.ModifyOrderRequest{Quantity: 5}); !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("quantity-only modify: err = %v, want ErrInvalidOrderRequest", err)
	}

	body, err := buildModifyOrderBody(broker.ModifyOrderRequest{Quantity: 5, Price: 70000})
	if err != nil {
		t.Fatal(err)
	}
	if body["orderType"] != "LIMIT" || body["price"] != "70000" {
		t.Fatalf("priced modify body = %v, want a LIMIT order at 70000", body)
	}

	body, err = buildModifyOrderBody(broker.ModifyOrderRequest{Type: broker.OrderTypeMarket, Quantity: 5})
	if err != nil {
		t.Fatal(err)
	}
	if body["orderType"] != "MARKET" {
		t.Fatalf("explicit market modify body = %v", body)
	}
}

func TestCreateOrderRejectsQuantityTogetherWithAmount(t *testing.T) {
	t.Parallel()
	_, err := buildCreateOrderBody(broker.OrderRequest{
		Symbol: "AAPL", Market: "US", Side: broker.OrderSideBuy, Type: broker.OrderTypeMarket,
		Quantity: 1, OrderAmount: 100,
	})
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("err = %v, want ErrInvalidOrderRequest", err)
	}
}

func TestReplacedOrderIsTerminal(t *testing.T) {
	t.Parallel()
	if got := mapOrderStatus("REPLACED"); got != broker.OrderStatusCancelled {
		t.Fatalf("REPLACED mapped to %q; the superseded order would look open forever", got)
	}
}

func TestNewClientOrderIDFitsTossFormat(t *testing.T) {
	t.Parallel()
	format := regexp.MustCompile(`^[A-Za-z0-9_-]{1,36}$`)
	first, second := newClientOrderID(), newClientOrderID()
	if !format.MatchString(first) || first == second {
		t.Fatalf("client order ids %q, %q", first, second)
	}
}
