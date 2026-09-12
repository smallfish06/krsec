package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/ls"
	"github.com/smallfish06/krsec/pkg/broker"
)

const maxOrderPages = 100

// CancelOrder cancels the currently unfilled quantity of a tracked same-day order.
func (a *Adapter) CancelOrder(ctx context.Context, orderID string) error {
	a.actionMu.Lock()
	defer a.actionMu.Unlock()
	meta, row, err := a.currentOrder(ctx, orderID)
	if err != nil {
		return err
	}
	remaining, err := orderInteger(row, "ordrem")
	if err != nil {
		return err
	}
	if remaining == 0 {
		return fmt.Errorf("%w: LS order has no remaining quantity", broker.ErrInvalidOrderRequest)
	}
	if _, err = a.trackedOrder(meta.OrderID); err != nil {
		return err
	}
	resp, err := a.client.CallEndpoint(ctx, "POST", ls.PathStockOrder, "CSPAT00801", map[string]any{
		"CSPAT00801InBlock1": map[string]any{"OrgOrdNo": anyInt64(meta.OrderID), "IsuNo": lsWireOrderSymbol(meta.Symbol, a.sandbox), "OrdQty": remaining},
	})
	if err != nil {
		return err
	}
	block, ok := mapValue(resp, "CSPAT00801OutBlock2")
	if !ok || normalizeOrderID(anyString(block["OrdNo"])) == "" {
		return fmt.Errorf("%w: LS cancel response missing order number; query status before retry", broker.ErrServerError)
	}
	// Acceptance is not confirmation that cancellation has completed. The
	// original context remains so callers can query the provider's final state.
	return nil
}

// ModifyOrder modifies the remaining quantity of a tracked same-day order.
func (a *Adapter) ModifyOrder(ctx context.Context, orderID string, req broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	a.actionMu.Lock()
	defer a.actionMu.Unlock()
	if req.QuantityDecimal != "" || req.Quantity < 0 || math.IsNaN(req.Price) || math.IsInf(req.Price, 0) || req.Price < 0 {
		return nil, broker.ErrInvalidOrderRequest
	}
	meta, row, err := a.currentOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	remaining, err := orderInteger(row, "ordrem")
	if err != nil {
		return nil, err
	}
	quantity := req.Quantity
	if quantity == 0 {
		quantity = remaining
	}
	if quantity <= 0 || quantity > remaining {
		return nil, fmt.Errorf("%w: LS modify quantity exceeds the unfilled quantity", broker.ErrInvalidOrderRequest)
	}
	orderType := req.Type
	if orderType == "" {
		orderType = meta.OrderType
	}
	typeCode, err := lsOrderType(orderType)
	if err != nil {
		return nil, err
	}
	price := req.Price
	if orderType == broker.OrderTypeLimit && price == 0 {
		price = anyFloat(row["price"])
	}
	if (orderType == broker.OrderTypeLimit && price <= 0) || (orderType == broker.OrderTypeMarket && price != 0) || math.IsNaN(price) || math.IsInf(price, 0) {
		return nil, broker.ErrInvalidOrderRequest
	}
	if _, err = a.trackedOrder(meta.OrderID); err != nil {
		return nil, err
	}
	resp, err := a.client.CallEndpoint(ctx, "POST", ls.PathStockOrder, "CSPAT00701", map[string]any{
		"CSPAT00701InBlock1": map[string]any{"OrgOrdNo": anyInt64(meta.OrderID), "IsuNo": lsWireOrderSymbol(meta.Symbol, a.sandbox), "OrdQty": quantity, "OrdprcPtnCode": typeCode, "OrdCndiTpCode": "0", "OrdPrc": price},
	})
	if err != nil {
		return nil, err
	}
	block, ok := mapValue(resp, "CSPAT00701OutBlock2")
	id := normalizeOrderID(anyString(block["OrdNo"]))
	if !ok || id == "" {
		return nil, fmt.Errorf("%w: LS modify response missing order number; query status before retry", broker.ErrServerError)
	}
	meta.OrderID = id
	meta.Quantity = quantity
	meta.Price = price
	meta.OrderType = orderType
	meta.PlacedAt = a.orderNow()
	a.storeOrderContext(id, meta)
	return &broker.OrderResult{OrderID: id, Status: broker.OrderStatusPending, RemainingQty: quantity, Timestamp: meta.PlacedAt, Message: anyString(resp["rsp_msg"])}, nil
}

// GetOrder reads a tracked same-day order. It does not infer fill average prices
// from the t0425 field named execution price; exact executions are available via GetOrderFills.
func (a *Adapter) GetOrder(ctx context.Context, orderID string) (*broker.OrderResult, error) {
	a.actionMu.Lock()
	defer a.actionMu.Unlock()
	meta, row, err := a.currentOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	quantity, err := orderInteger(row, "qty")
	if err != nil {
		return nil, err
	}
	filled, err := orderInteger(row, "cheqty")
	if err != nil {
		return nil, err
	}
	remaining, err := orderInteger(row, "ordrem")
	if err != nil {
		return nil, err
	}
	status := broker.OrderStatusPending
	rawStatus := anyString(row["status"])
	switch {
	case strings.Contains(rawStatus, "거부") || strings.Contains(rawStatus, "거절"):
		status = broker.OrderStatusRejected
	case remaining == 0 && strings.Contains(rawStatus, "취소"):
		status = broker.OrderStatusCancelled
	case quantity > 0 && filled >= quantity:
		status = broker.OrderStatusFilled
	}
	timestamp := orderClock(meta.PlacedAt, anyString(row["ordtime"]))
	if timestamp.IsZero() {
		timestamp = meta.PlacedAt
	}
	return &broker.OrderResult{OrderID: meta.OrderID, Status: status, FilledQuantity: filled, RemainingQty: remaining, Timestamp: timestamp, Message: rawStatus}, nil
}

// GetOrderFills returns exact reported executions for a tracked same-day order.
func (a *Adapter) GetOrderFills(ctx context.Context, orderID string) ([]broker.OrderFill, error) {
	a.actionMu.Lock()
	defer a.actionMu.Unlock()
	meta, err := a.trackedOrder(orderID)
	if err != nil {
		return nil, err
	}
	var continuation ls.Continuation
	seen := map[string]bool{}
	seenExecutions := map[string]bool{}
	var fills []broker.OrderFill
	date := meta.PlacedAt.In(lsOrderLocation).Format("20060102")
	for pageNumber := 0; pageNumber < maxOrderPages; pageNumber++ {
		page, callErr := a.client.CallEndpointPage(ctx, "POST", ls.PathStockAccount, "CSPAQ13700", map[string]any{
			"CSPAQ13700InBlock1": map[string]any{"OrdMktCode": "00", "BnsTpCode": "0", "IsuNo": "A" + meta.Symbol, "ExecYn": "1", "OrdDt": date, "SrtOrdNo2": 0, "BkseqTpCode": "1", "OrdPtnCode": "00"},
		}, continuation)
		if callErr != nil {
			return nil, callErr
		}
		rows, rowsErr := orderRows(page.Data, "CSPAQ13700OutBlock3")
		if rowsErr != nil {
			return nil, rowsErr
		}
		for _, row := range rows {
			if normalizeOrderID(anyString(row["OrdNo"])) != meta.OrderID {
				continue
			}
			if anyString(row["OrdDt"]) != date || canonicalOrderSymbol(anyString(row["IsuNo"])) != meta.Symbol || anyString(row["BnsTpCode"]) != mustOrderSide(meta.Side) {
				return nil, fmt.Errorf("%w: LS fill identity differs from recorded order", broker.ErrServerError)
			}
			quantity, qtyErr := orderInteger(row, "ExecQty")
			if qtyErr != nil {
				return nil, qtyErr
			}
			if quantity == 0 {
				continue
			}
			price := anyFloat(row["ExecPrc"])
			if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
				return nil, fmt.Errorf("%w: LS fill price unavailable", broker.ErrServerError)
			}
			filledAt := orderClock(meta.PlacedAt, anyString(row["ExecTrxTime"]))
			if filledAt.IsZero() {
				return nil, fmt.Errorf("%w: LS execution time unavailable", broker.ErrServerError)
			}
			encoded, _ := json.Marshal(row)
			if seenExecutions[string(encoded)] {
				return nil, fmt.Errorf("%w: duplicate LS execution cannot be identified uniquely", broker.ErrServerError)
			}
			seenExecutions[string(encoded)] = true
			fills = append(fills, broker.OrderFill{OrderID: meta.OrderID, Symbol: meta.Symbol, Market: meta.Market, Side: string(meta.Side), Quantity: quantity, Price: price, Amount: float64(quantity) * price, Currency: "KRW", FilledAt: filledAt, RawStatus: anyString(row["OrdTrxPtnNm"])})
		}
		next, more, nextErr := nextOrderContinuation(page.Continuation, "", seen)
		if nextErr != nil {
			return nil, nextErr
		}
		if !more {
			return fills, nil
		}
		continuation = next
	}
	return nil, fmt.Errorf("%w: LS fill pagination exceeded limit", broker.ErrServerError)
}

func (a *Adapter) currentOrder(ctx context.Context, id string) (orderContext, map[string]any, error) {
	meta, err := a.trackedOrder(id)
	if err != nil {
		return orderContext{}, nil, err
	}
	var continuation ls.Continuation
	cursor := ""
	seen := map[string]bool{}
	var found map[string]any
	var foundJSON string
	for pageNumber := 0; pageNumber < maxOrderPages; pageNumber++ {
		page, callErr := a.client.CallEndpointPage(ctx, "POST", ls.PathStockAccount, "t0425", map[string]any{
			"t0425InBlock": map[string]any{"expcode": meta.Symbol, "chegb": "0", "medosu": "0", "sortgb": "1", "cts_ordno": cursor},
		}, continuation)
		if callErr != nil {
			return orderContext{}, nil, callErr
		}
		rows, rowsErr := orderRows(page.Data, "t0425OutBlock1")
		if rowsErr != nil {
			return orderContext{}, nil, rowsErr
		}
		for _, row := range rows {
			if normalizeOrderID(anyString(row["ordno"])) != meta.OrderID {
				continue
			}
			if canonicalOrderSymbol(anyString(row["expcode"])) != meta.Symbol {
				return orderContext{}, nil, fmt.Errorf("%w: LS order symbol differs from recorded order", broker.ErrInvalidOrderRequest)
			}
			if side := anyString(row["medosu"]); side != mustOrderSide(meta.Side) && side != orderSideName(meta.Side) {
				return orderContext{}, nil, fmt.Errorf("%w: LS order side differs from recorded order", broker.ErrInvalidOrderRequest)
			}
			encoded, _ := json.Marshal(row)
			if found != nil && foundJSON != string(encoded) {
				return orderContext{}, nil, fmt.Errorf("%w: ambiguous LS order history", broker.ErrInvalidOrderRequest)
			}
			found = row
			foundJSON = string(encoded)
		}
		block, _ := mapValue(page.Data, "t0425OutBlock")
		nextCursor := anyString(block["cts_ordno"])
		next, more, nextErr := nextOrderContinuation(page.Continuation, nextCursor, seen)
		if nextErr != nil {
			return orderContext{}, nil, nextErr
		}
		if !more {
			if _, dateErr := a.trackedOrder(meta.OrderID); dateErr != nil {
				return orderContext{}, nil, dateErr
			}
			if found == nil {
				return orderContext{}, nil, broker.ErrOrderNotFound
			}
			quantity, qtyErr := orderInteger(found, "qty")
			filled, fillErr := orderInteger(found, "cheqty")
			remaining, remainingErr := orderInteger(found, "ordrem")
			if qtyErr != nil || fillErr != nil || remainingErr != nil || quantity == 0 || filled > quantity || remaining > quantity-filled {
				return orderContext{}, nil, fmt.Errorf("%w: inconsistent LS order quantities", broker.ErrServerError)
			}
			return meta, found, nil
		}
		cursor = nextCursor
		continuation = next
	}
	return orderContext{}, nil, fmt.Errorf("%w: LS order pagination exceeded limit", broker.ErrServerError)
}

func nextOrderContinuation(continuation ls.Continuation, cursor string, seen map[string]bool) (ls.Continuation, bool, error) {
	flag := strings.ToUpper(strings.TrimSpace(continuation.TRCont))
	key := strings.TrimSpace(continuation.TRContKey)
	cursor = strings.TrimSpace(cursor)
	if cursor == "0" {
		cursor = ""
	}
	if flag != "" && flag != "N" && flag != "Y" {
		return ls.Continuation{}, false, fmt.Errorf("%w: invalid LS continuation flag", broker.ErrServerError)
	}
	if flag == "Y" && key == "" {
		return ls.Continuation{}, false, fmt.Errorf("%w: LS continuation key missing", broker.ErrServerError)
	}
	if flag != "Y" && cursor == "" {
		return ls.Continuation{}, false, nil
	}
	next := ls.Continuation{}
	if flag == "Y" {
		next = ls.Continuation{TRCont: "Y", TRContKey: key}
	}
	identity := cursor + "|" + next.TRCont + "|" + next.TRContKey
	if seen[identity] {
		return ls.Continuation{}, false, fmt.Errorf("%w: repeated LS continuation", broker.ErrServerError)
	}
	seen[identity] = true
	return next, true, nil
}

func orderRows(data map[string]any, key string) ([]map[string]any, error) {
	if data[key] == nil {
		return nil, nil
	}
	items, ok := data[key].([]any)
	if !ok {
		return nil, fmt.Errorf("%w: invalid LS %s rows", broker.ErrServerError, key)
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: invalid LS order row", broker.ErrServerError)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func orderInteger(row map[string]any, key string) (int64, error) {
	quantity, err := strconv.ParseInt(anyString(row[key]), 10, 64)
	if err != nil || quantity < 0 {
		return 0, fmt.Errorf("%w: invalid LS order field %s", broker.ErrServerError, key)
	}
	return quantity, nil
}

func orderClock(date time.Time, clock string) time.Time {
	clock = strings.ReplaceAll(strings.TrimSpace(clock), ":", "")
	if len(clock) == 9 {
		clock = clock[:6] + "." + clock[6:]
	}
	timestamp, err := time.ParseInLocation("20060102150405.999", date.In(lsOrderLocation).Format("20060102")+clock, lsOrderLocation)
	if err != nil {
		return time.Time{}
	}
	return timestamp
}

func mustOrderSide(side broker.OrderSide) string { code, _ := lsOrderSide(side); return code }
func orderSideName(side broker.OrderSide) string {
	if side == broker.OrderSideBuy {
		return "매수"
	}
	if side == broker.OrderSideSell {
		return "매도"
	}
	return ""
}
