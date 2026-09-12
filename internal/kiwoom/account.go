package kiwoom

import (
	"context"
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

// InquireBalanceByRequest fetches kt00005.
func (c *Client) InquireBalanceByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKt00005Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00005Response, error) {
	req.DmstStexTp = strings.ToUpper(strings.TrimSpace(req.DmstStexTp))
	if req.DmstStexTp == "" {
		req.DmstStexTp = "KRX"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00005", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00005Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireBalance fetches kt00005.
func (c *Client) InquireBalance(ctx context.Context, exchange string) (*kiwoomspecs.KiwoomApiDostkAcntKt00005Response, error) {
	return c.InquireBalanceByRequest(ctx, kiwoomspecs.KiwoomApiDostkAcntKt00005Request{
		DmstStexTp: exchange,
	})
}

// InquirePositionsByRequest fetches kt00018.
func (c *Client) InquirePositionsByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKt00018Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00018Response, error) {
	req.QryTp = strings.TrimSpace(req.QryTp)
	if req.QryTp == "" {
		req.QryTp = "1"
	}
	if req.QryTp != "1" && req.QryTp != "2" {
		return nil, fmt.Errorf("%w: kt00018 qry_tp must be 1 or 2", broker.ErrInvalidOrderRequest)
	}
	req.DmstStexTp = strings.ToUpper(strings.TrimSpace(req.DmstStexTp))
	if req.DmstStexTp == "" {
		req.DmstStexTp = "KRX"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00018", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00018Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquirePositions fetches kt00018.
func (c *Client) InquirePositions(ctx context.Context, queryType, exchange string) (*kiwoomspecs.KiwoomApiDostkAcntKt00018Response, error) {
	return c.InquirePositionsByRequest(ctx, kiwoomspecs.KiwoomApiDostkAcntKt00018Request{
		QryTp:      queryType,
		DmstStexTp: exchange,
	})
}

// InquireUnsettledOrdersByRequest fetches ka10075.
func (c *Client) InquireUnsettledOrdersByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKa10075Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKa10075Response, error) {
	req.AllStkTp = strings.TrimSpace(req.AllStkTp)
	req.TrdeTp = strings.TrimSpace(req.TrdeTp)
	req.StexTp = strings.TrimSpace(req.StexTp)
	if req.TrdeTp == "" {
		req.TrdeTp = "0"
	}
	if req.StexTp == "" {
		req.StexTp = "0"
	}
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		if req.AllStkTp == "" {
			req.AllStkTp = "0"
		}
	} else {
		req.AllStkTp = "1"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10075", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKa10075Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireUnsettledOrders fetches ka10075.
func (c *Client) InquireUnsettledOrders(ctx context.Context, symbol string) (*kiwoomspecs.KiwoomApiDostkAcntKa10075Response, error) {
	return c.InquireUnsettledOrdersByRequest(ctx, kiwoomspecs.KiwoomApiDostkAcntKa10075Request{
		StkCd: symbol,
	})
}

// InquireOrderExecutionsByRequest fetches ka10076.
func (c *Client) InquireOrderExecutionsByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKa10076Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKa10076Response, error) {
	req.QryTp = strings.TrimSpace(req.QryTp)
	req.SellTp = strings.TrimSpace(req.SellTp)
	req.StexTp = strings.TrimSpace(req.StexTp)
	if req.QryTp == "" {
		req.QryTp = "0"
	}
	if req.SellTp == "" {
		req.SellTp = "0"
	}
	if req.StexTp == "" {
		req.StexTp = "0"
	}
	req.StkCd = normalizeSymbolCode(req.StkCd)

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10076", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKa10076Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireOrderExecutions fetches ka10076.
func (c *Client) InquireOrderExecutions(ctx context.Context, symbol string) (*kiwoomspecs.KiwoomApiDostkAcntKa10076Response, error) {
	return c.InquireOrderExecutionsByRequest(ctx, kiwoomspecs.KiwoomApiDostkAcntKa10076Request{
		StkCd: symbol,
	})
}

// InquireOrderExecutionDetail fetches kt00007.
func (c *Client) InquireOrderExecutionDetail(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKt00007Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00007Response, error) {
	req.QryTp = strings.TrimSpace(req.QryTp)
	req.StkBondTp = strings.TrimSpace(req.StkBondTp)
	req.SellTp = strings.TrimSpace(req.SellTp)
	req.DmstStexTp = strings.ToUpper(strings.TrimSpace(req.DmstStexTp))
	if req.QryTp == "" {
		req.QryTp = "0"
	}
	if req.StkBondTp == "" {
		req.StkBondTp = "0"
	}
	if req.SellTp == "" {
		req.SellTp = "0"
	}
	if req.DmstStexTp == "" {
		req.DmstStexTp = "KRX"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00007", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00007Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireOrderExecutionStatus fetches kt00009.
func (c *Client) InquireOrderExecutionStatus(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKt00009Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00009Response, error) {
	req.QryTp = strings.TrimSpace(req.QryTp)
	req.StkBondTp = strings.TrimSpace(req.StkBondTp)
	req.SellTp = strings.TrimSpace(req.SellTp)
	req.MrktTp = strings.TrimSpace(req.MrktTp)
	req.DmstStexTp = strings.ToUpper(strings.TrimSpace(req.DmstStexTp))
	if req.QryTp == "" {
		req.QryTp = "0"
	}
	if req.StkBondTp == "" {
		req.StkBondTp = "0"
	}
	if req.SellTp == "" {
		req.SellTp = "0"
	}
	if req.MrktTp == "" {
		req.MrktTp = "000"
	}
	if req.DmstStexTp == "" {
		req.DmstStexTp = "KRX"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00009", PathAccount, &req, 20)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00009Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireOrderableWithdrawable fetches kt00010.
func (c *Client) InquireOrderableWithdrawable(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkAcntKt00010Request,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00010Response, error) {
	req.TrdeTp = strings.TrimSpace(req.TrdeTp)
	req.Uv = strings.TrimSpace(req.Uv)
	if req.TrdeTp == "" {
		req.TrdeTp = "0"
	}
	if req.Uv == "" {
		req.Uv = "0"
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00010", PathAccount, &req, 20)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00010Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquirePositionsByAsset fetches positions with optional stock/bond filter (e.g. 1=stock, 2=bond).
func (c *Client) InquirePositionsByAsset(
	ctx context.Context,
	queryType, exchange, stockBondType string,
) (*kiwoomspecs.KiwoomApiDostkAcntKt00018Response, error) {
	queryType = strings.TrimSpace(queryType)
	if queryType == "" {
		queryType = "1"
	}
	if queryType != "1" && queryType != "2" {
		return nil, fmt.Errorf("%w: kt00018 qry_tp must be 1 or 2", broker.ErrInvalidOrderRequest)
	}
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	if exchange == "" {
		exchange = "KRX"
	}

	req := struct {
		kiwoomspecs.KiwoomApiDostkAcntKt00018Request
		StkBondTp string `json:"stk_bond_tp,omitempty"`
	}{
		KiwoomApiDostkAcntKt00018Request: kiwoomspecs.KiwoomApiDostkAcntKt00018Request{
			QryTp:      queryType,
			DmstStexTp: exchange,
		},
		StkBondTp: strings.TrimSpace(stockBondType),
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "kt00018", PathAccount, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkAcntKt00018Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireBondPositions is a convenience wrapper for bond holdings.
func (c *Client) InquireBondPositions(ctx context.Context, exchange string) (*kiwoomspecs.KiwoomApiDostkAcntKt00018Response, error) {
	return c.InquirePositionsByAsset(ctx, "1", exchange, "2")
}

// InquireUnsettledOrdersByExchange fetches unsettled orders with explicit exchange scope.
func (c *Client) InquireUnsettledOrdersByExchange(
	ctx context.Context,
	symbol, exchangeType string,
) (*kiwoomspecs.KiwoomApiDostkAcntKa10075Response, error) {
	req := kiwoomspecs.KiwoomApiDostkAcntKa10075Request{
		StexTp: strings.TrimSpace(exchangeType),
	}
	req.StkCd = symbol
	return c.InquireUnsettledOrdersByRequest(ctx, req)
}

// InquireOrderExecutionsByExchange fetches execution rows with explicit exchange scope.
func (c *Client) InquireOrderExecutionsByExchange(
	ctx context.Context,
	symbol, exchangeType string,
) (*kiwoomspecs.KiwoomApiDostkAcntKa10076Response, error) {
	req := kiwoomspecs.KiwoomApiDostkAcntKa10076Request{
		StexTp: strings.TrimSpace(exchangeType),
	}
	req.StkCd = symbol
	return c.InquireOrderExecutionsByRequest(ctx, req)
}
