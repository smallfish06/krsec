package kiwoom

import (
	"context"
	"strings"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

// InquireDailyPriceByRequest fetches daily candles from ka10081.
func (c *Client) InquireDailyPriceByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkChartKa10081Request,
) (*kiwoomspecs.KiwoomApiDostkChartKa10081Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	req.BaseDt = strings.TrimSpace(req.BaseDt)
	if req.BaseDt == "" {
		req.BaseDt = time.Now().Format("20060102")
	}
	setDefaultString(&req.UpdStkpcTp, "1")

	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10081", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10081Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireDailyPrice fetches daily candles from ka10081.
func (c *Client) InquireDailyPrice(
	ctx context.Context,
	symbol, baseDate string,
) (*kiwoomspecs.KiwoomApiDostkChartKa10081Response, error) {
	return c.InquireDailyPriceByRequest(ctx, kiwoomspecs.KiwoomApiDostkChartKa10081Request{
		StkCd:  symbol,
		BaseDt: baseDate,
	})
}

// InquireWeeklyPriceByRequest fetches weekly candles from ka10082.
func (c *Client) InquireWeeklyPriceByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkChartKa10082Request,
) (*kiwoomspecs.KiwoomApiDostkChartKa10082Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	req.BaseDt = strings.TrimSpace(req.BaseDt)
	if req.BaseDt == "" {
		req.BaseDt = time.Now().Format("20060102")
	}
	setDefaultString(&req.UpdStkpcTp, "1")

	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10082", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10082Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireWeeklyPrice fetches weekly candles from ka10082.
func (c *Client) InquireWeeklyPrice(
	ctx context.Context,
	symbol, baseDate string,
) (*kiwoomspecs.KiwoomApiDostkChartKa10082Response, error) {
	return c.InquireWeeklyPriceByRequest(ctx, kiwoomspecs.KiwoomApiDostkChartKa10082Request{
		StkCd:  symbol,
		BaseDt: baseDate,
	})
}

// InquireMonthlyPriceByRequest fetches monthly candles from ka10083.
func (c *Client) InquireMonthlyPriceByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkChartKa10083Request,
) (*kiwoomspecs.KiwoomApiDostkChartKa10083Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	req.BaseDt = strings.TrimSpace(req.BaseDt)
	if req.BaseDt == "" {
		req.BaseDt = time.Now().Format("20060102")
	}
	setDefaultString(&req.UpdStkpcTp, "1")

	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10083", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10083Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireMonthlyPrice fetches monthly candles from ka10083.
func (c *Client) InquireMonthlyPrice(
	ctx context.Context,
	symbol, baseDate string,
) (*kiwoomspecs.KiwoomApiDostkChartKa10083Response, error) {
	return c.InquireMonthlyPriceByRequest(ctx, kiwoomspecs.KiwoomApiDostkChartKa10083Request{
		StkCd:  symbol,
		BaseDt: baseDate,
	})
}

// InquireTickChartByRequest fetches domestic tick chart via ka10079.
func (c *Client) InquireTickChartByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkChartKa10079Request,
) (*kiwoomspecs.KiwoomApiDostkChartKa10079Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	setDefaultString(&req.TicScope, "1")
	setDefaultString(&req.UpdStkpcTp, "1")

	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10079", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10079Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireTickChart fetches domestic tick chart via ka10079.
func (c *Client) InquireTickChart(
	ctx context.Context,
	symbol, baseDate string,
) (*kiwoomspecs.KiwoomApiDostkChartKa10079Response, error) {
	symbol = normalizeSymbolCode(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	baseDate = strings.TrimSpace(baseDate)
	if baseDate == "" {
		baseDate = time.Now().Format("20060102")
	}
	req := struct {
		kiwoomspecs.KiwoomApiDostkChartKa10079Request
		BaseDt string `json:"base_dt,omitempty"`
	}{
		StkCd:      symbol,
		TicScope:   "1",
		UpdStkpcTp: "1",
		BaseDt:     baseDate,
	}
	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10079", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10079Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireInvestorByStockChart fetches investor trend chart via ka10060.
func (c *Client) InquireInvestorByStockChart(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkChartKa10060Request,
) (*kiwoomspecs.KiwoomApiDostkChartKa10060Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	setDefaultString(&req.Dt, time.Now().Format("20060102"))
	setDefaultString(&req.TrdeTp, "0")
	setDefaultString(&req.AmtQtyTp, "1")
	setDefaultString(&req.UnitTp, "1")

	respObj, err := c.CallDocumentedEndpoint(ctx, "ka10060", PathChart, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkChartKa10060Response{}
	if err := bindResponseObject(respObj, out); err != nil {
		return nil, err
	}
	return out, nil
}
