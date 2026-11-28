package kiwoom

import (
	"context"
	"github.com/smallfish06/krsec/internal/krtime"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

// InquireExecutionInfo fetches ka10003.
func (c *Client) InquireExecutionInfo(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkStkinfoKa10003Request,
) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10003Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10003", PathStockInfo, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkStkinfoKa10003Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireOrderBook fetches ka10004.
func (c *Client) InquireOrderBook(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkMrkcondKa10004Request,
) (*kiwoomspecs.KiwoomApiDostkMrkcondKa10004Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10004", PathMarketCond, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkMrkcondKa10004Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireInvestorByStock fetches ka10059.
func (c *Client) InquireInvestorByStock(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkStkinfoKa10059Request,
) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10059Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}
	setDefaultString(&req.Dt, krtime.Today())
	setDefaultString(&req.TrdeTp, "0")
	setDefaultString(&req.AmtQtyTp, "1")
	setDefaultString(&req.UnitTp, "1")

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10059", PathStockInfo, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkStkinfoKa10059Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireVolumeRank fetches ka10030.
func (c *Client) InquireVolumeRank(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkRkinfoKa10030Request,
) (*kiwoomspecs.KiwoomApiDostkRkinfoKa10030Response, error) {
	setDefaultString(&req.StexTp, "0")
	setDefaultString(&req.MrktTp, "000")
	setDefaultString(&req.SortTp, "1")
	setDefaultString(&req.MangStkIncls, "0")
	setDefaultString(&req.CrdTp, "0")
	setDefaultString(&req.TrdeQtyTp, "0")
	setDefaultString(&req.PricTp, "0")
	setDefaultString(&req.TrdePricaTp, "0")
	setDefaultString(&req.MrktOpenTp, "0")

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10030", PathRankingInfo, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkRkinfoKa10030Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireChangeRateRank fetches ka10027.
func (c *Client) InquireChangeRateRank(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkRkinfoKa10027Request,
) (*kiwoomspecs.KiwoomApiDostkRkinfoKa10027Response, error) {
	setDefaultString(&req.StexTp, "0")
	setDefaultString(&req.MrktTp, "000")
	setDefaultString(&req.SortTp, "1")
	setDefaultString(&req.TrdeQtyCnd, "0")
	setDefaultString(&req.StkCnd, "0")
	setDefaultString(&req.CrdCnd, "0")
	setDefaultString(&req.UpdownIncls, "1")
	setDefaultString(&req.PricCnd, "0")
	setDefaultString(&req.TrdePricaCnd, "0")

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10027", PathRankingInfo, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkRkinfoKa10027Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireSectorCurrent fetches ka20001.
func (c *Client) InquireSectorCurrent(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkSectKa20001Request,
) (*kiwoomspecs.KiwoomApiDostkSectKa20001Response, error) {
	setDefaultString(&req.MrktTp, "000")

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka20001", PathSector, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkSectKa20001Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}

// InquireSectorByPrice fetches ka20002.
func (c *Client) InquireSectorByPrice(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkSectKa20002Request,
) (*kiwoomspecs.KiwoomApiDostkSectKa20002Response, error) {
	setDefaultString(&req.MrktTp, "000")
	setDefaultString(&req.StexTp, "0")

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka20002", PathSector, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkSectKa20002Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}
	return out, nil
}
