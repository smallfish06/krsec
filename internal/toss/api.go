package toss

import (
	"context"
	"net/http"
	"strings"
)

func (c *Client) GetAccounts(ctx context.Context) ([]Account, error) {
	var env apiEnvelope[[]Account]
	err := c.callJSON(ctx, http.MethodGet, PathAccounts, "", nil, nil, &env)
	return env.Result, err
}

func (c *Client) GetPrices(ctx context.Context, symbols ...string) ([]PriceResponse, error) {
	var env apiEnvelope[[]PriceResponse]
	err := c.callJSON(ctx, http.MethodGet, PathPrices, "", map[string]any{
		"symbols": strings.Join(cleanSymbols(symbols), ","),
	}, nil, &env)
	return env.Result, err
}

func (c *Client) GetCandles(ctx context.Context, symbol, interval string, count int, before string, adjusted bool) (CandlePageResponse, error) {
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}
	query := map[string]any{
		"symbol":   strings.TrimSpace(symbol),
		"interval": strings.TrimSpace(interval),
		"count":    count,
		"adjusted": adjusted,
	}
	if strings.TrimSpace(before) != "" {
		query["before"] = strings.TrimSpace(before)
	}
	var env apiEnvelope[CandlePageResponse]
	err := c.callJSON(ctx, http.MethodGet, PathCandles, "", query, nil, &env)
	return env.Result, err
}

func (c *Client) GetStocks(ctx context.Context, symbols ...string) ([]StockInfo, error) {
	var env apiEnvelope[[]StockInfo]
	err := c.callJSON(ctx, http.MethodGet, PathStocks, "", map[string]any{
		"symbols": strings.Join(cleanSymbols(symbols), ","),
	}, nil, &env)
	return env.Result, err
}

func (c *Client) GetHoldings(ctx context.Context, accountSeq, symbol string) (HoldingsOverview, error) {
	query := map[string]any{}
	if strings.TrimSpace(symbol) != "" {
		query["symbol"] = strings.TrimSpace(symbol)
	}
	var env apiEnvelope[HoldingsOverview]
	err := c.callJSON(ctx, http.MethodGet, PathHoldings, accountSeq, query, nil, &env)
	return env.Result, err
}

func (c *Client) GetBuyingPower(ctx context.Context, accountSeq, currency string) (BuyingPowerResponse, error) {
	var env apiEnvelope[BuyingPowerResponse]
	err := c.callJSON(ctx, http.MethodGet, PathBuyingPower, accountSeq, map[string]any{
		"currency": strings.ToUpper(strings.TrimSpace(currency)),
	}, nil, &env)
	return env.Result, err
}

func (c *Client) GetSellableQuantity(ctx context.Context, accountSeq, symbol string) (SellableQuantityResponse, error) {
	var env apiEnvelope[SellableQuantityResponse]
	err := c.callJSON(ctx, http.MethodGet, PathSellableQuantity, accountSeq, map[string]any{
		"symbol": strings.TrimSpace(symbol),
	}, nil, &env)
	return env.Result, err
}

func (c *Client) GetCommissions(ctx context.Context, accountSeq string) ([]Commission, error) {
	var env apiEnvelope[[]Commission]
	err := c.callJSON(ctx, http.MethodGet, PathCommissions, accountSeq, nil, nil, &env)
	return env.Result, err
}

func (c *Client) CreateOrder(ctx context.Context, accountSeq string, body map[string]any) (OrderResponse, error) {
	var env apiEnvelope[OrderResponse]
	err := c.callJSON(ctx, http.MethodPost, PathOrders, accountSeq, nil, body, &env)
	return env.Result, err
}

func (c *Client) ModifyOrder(ctx context.Context, accountSeq, orderID string, body map[string]any) (OrderOperationResponse, error) {
	var env apiEnvelope[OrderOperationResponse]
	err := c.callJSON(ctx, http.MethodPost, PathOrders+"/"+strings.TrimSpace(orderID)+"/modify", accountSeq, nil, body, &env)
	return env.Result, err
}

func (c *Client) CancelOrder(ctx context.Context, accountSeq, orderID string) (OrderOperationResponse, error) {
	var env apiEnvelope[OrderOperationResponse]
	err := c.callJSON(ctx, http.MethodPost, PathOrders+"/"+strings.TrimSpace(orderID)+"/cancel", accountSeq, nil, map[string]any{}, &env)
	return env.Result, err
}

func (c *Client) GetOrder(ctx context.Context, accountSeq, orderID string) (Order, error) {
	var env apiEnvelope[Order]
	err := c.callJSON(ctx, http.MethodGet, PathOrders+"/"+strings.TrimSpace(orderID), accountSeq, nil, nil, &env)
	return env.Result, err
}

func (c *Client) GetOrders(ctx context.Context, accountSeq string, query map[string]any) (PaginatedOrderResponse, error) {
	var env apiEnvelope[PaginatedOrderResponse]
	err := c.callJSON(ctx, http.MethodGet, PathOrders, accountSeq, query, nil, &env)
	return env.Result, err
}

func cleanSymbols(symbols []string) []string {
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if symbol != "" {
			out = append(out, symbol)
		}
	}
	return out
}
