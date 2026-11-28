// Package ordercheck validates common order requests on behalf of adapters
// whose broker API cannot express every field of broker.OrderRequest.
package ordercheck

import (
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
)

// Side returns nil only for the two known sides. Adapters map the side onto a
// broker code with a default branch; without this check an unrecognized value
// would be sent as whichever side that default is.
func Side(side broker.OrderSide) error {
	if side == broker.OrderSideBuy || side == broker.OrderSideSell {
		return nil
	}
	return fmt.Errorf("%w: side must be %q or %q, got %q",
		broker.ErrInvalidOrderRequest, broker.OrderSideBuy, broker.OrderSideSell, side)
}

// WholeShareDayOrder rejects the request options that only some brokers
// support: fractional or amount-based quantities and time-in-force other than
// a day order. Dropping them silently would place a different order than the
// caller asked for.
func WholeShareDayOrder(brokerName string, req broker.OrderRequest) error {
	unsupported := func(field string) error {
		return fmt.Errorf("%w: %s does not support %s", broker.ErrInvalidOrderRequest, brokerName, field)
	}
	switch {
	case strings.TrimSpace(req.QuantityDecimal) != "":
		return unsupported("quantity_decimal")
	case req.OrderAmount != 0 || strings.TrimSpace(req.OrderAmountDecimal) != "":
		return unsupported("order_amount")
	}
	if tif := strings.ToLower(strings.TrimSpace(req.TimeInForce)); tif != "" && tif != "day" {
		return unsupported("time_in_force " + req.TimeInForce)
	}
	return nil
}
