package server

import (
	"math/big"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

func (s *Server) handleGetBalance(c fuego.ContextNoBody) (Response, error) {
	accountID := c.PathParam("account_id")

	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{
			OK:    false,
			Error: reason,
		})
	}

	balance, err := brk.GetBalance(c.Context(), accountID)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:    false,
			Error: err.Error(),
		})
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   balance,
		Broker: brk.Name(),
	})
}

func (s *Server) handleGetPositions(c fuego.ContextNoBody) (Response, error) {
	accountID := c.PathParam("account_id")

	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if brk == nil {
		return respond(c, status, Response{
			OK:    false,
			Error: reason,
		})
	}

	positions, err := brk.GetPositions(c.Context(), accountID)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:    false,
			Error: err.Error(),
		})
	}

	if c.QueryParam("group") == "true" {
		positions = groupPositions(positions)
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   positions,
		Broker: brk.Name(),
	})
}

// groupPositions merges rows that share a symbol and asset type, such as the
// cash and margin lots of one stock, keeping the order rows first appeared in.
func groupPositions(positions []broker.Position) []broker.Position {
	type group struct {
		pos  broker.Position
		cost float64
	}
	groups := make([]*group, 0, len(positions))
	byKey := make(map[string]*group, len(positions))

	for _, p := range positions {
		key := p.Symbol + "|" + string(p.AssetType)
		g, ok := byKey[key]
		if !ok {
			g = &group{pos: p, cost: positionCost(p)}
			byKey[key] = g
			groups = append(groups, g)
			continue
		}

		g.cost += positionCost(p)
		g.pos.QuantityDecimal = addDecimalQuantity(g.pos.QuantityDecimal, g.pos.Quantity, p.QuantityDecimal, p.Quantity)
		g.pos.OrderableQtyDecimal = addDecimalQuantity(g.pos.OrderableQtyDecimal, g.pos.OrderableQty, p.OrderableQtyDecimal, p.OrderableQty)
		g.pos.Quantity += p.Quantity
		g.pos.OrderableQty += p.OrderableQty
		g.pos.UnsettledQty += p.UnsettledQty
		g.pos.TodayBuyQty += p.TodayBuyQty
		g.pos.TodaySellQty += p.TodaySellQty
		g.pos.PurchaseValue += p.PurchaseValue
		g.pos.MarketValue += p.MarketValue
		g.pos.ProfitLoss += p.ProfitLoss
		g.pos.WeightPct += p.WeightPct
		if g.pos.MarketCode != p.MarketCode {
			g.pos.MarketCode = ""
		}
		if g.pos.LoanDate != p.LoanDate {
			g.pos.LoanDate = ""
		}
	}

	result := make([]broker.Position, 0, len(groups))
	for _, g := range groups {
		// Leave the first row's figures in place when there is nothing to
		// divide by: a zero quantity would otherwise yield NaN, which
		// encoding/json refuses to marshal.
		if qty := positionQuantity(g.pos); qty > 0 && g.cost > 0 {
			g.pos.AvgPrice = g.cost / qty
			g.pos.ProfitLossPct = g.pos.ProfitLoss / g.cost * 100
		}
		result = append(result, g.pos)
	}
	return result
}

// positionQuantity prefers the decimal quantity, which is authoritative for
// fractional holdings whose integer field is truncated.
func positionQuantity(p broker.Position) float64 {
	if r, ok := new(big.Rat).SetString(strings.TrimSpace(p.QuantityDecimal)); ok {
		qty, _ := r.Float64()
		return qty
	}
	return float64(p.Quantity)
}

func positionCost(p broker.Position) float64 {
	if p.PurchaseValue != 0 {
		return p.PurchaseValue
	}
	return p.AvgPrice * positionQuantity(p)
}

// addDecimalQuantity sums two quantities exactly. It returns "" when neither
// side carries a decimal string, leaving the integer field as the only value.
func addDecimalQuantity(aDecimal string, aInt int64, bDecimal string, bInt int64) string {
	aDecimal, bDecimal = strings.TrimSpace(aDecimal), strings.TrimSpace(bDecimal)
	if aDecimal == "" && bDecimal == "" {
		return ""
	}
	parse := func(decimal string, whole int64) *big.Rat {
		if r, ok := new(big.Rat).SetString(decimal); ok {
			return r
		}
		return new(big.Rat).SetInt64(whole)
	}
	sum := new(big.Rat).Add(parse(aDecimal, aInt), parse(bDecimal, bInt))
	if sum.IsInt() {
		return sum.Num().String()
	}
	return strings.TrimRight(sum.FloatString(12), "0")
}
