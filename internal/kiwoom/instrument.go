package kiwoom

import (
	"context"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

// InquireInstrumentInfoByRequest fetches ka10100.
func (c *Client) InquireInstrumentInfoByRequest(
	ctx context.Context,
	req kiwoomspecs.KiwoomApiDostkStkinfoKa10100Request,
) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10100Response, error) {
	req.StkCd = normalizeSymbolCode(req.StkCd)
	if req.StkCd == "" {
		return nil, broker.ErrInvalidSymbol
	}

	resObj, err := c.CallDocumentedEndpoint(ctx, "ka10100", PathStockInfo, &req)
	if err != nil {
		return nil, err
	}
	out := &kiwoomspecs.KiwoomApiDostkStkinfoKa10100Response{}
	if err := bindResponseObject(resObj, out); err != nil {
		return nil, err
	}

	// Some upstream payloads still use camelCase keys that can bypass exact tag names.
	// Fill missing fields via map fallback to preserve compatibility.
	if strings.TrimSpace(out.Code) == "" {
		res, err := responseBodyMap(resObj)
		if err != nil {
			return nil, err
		}
		out.Code = asString(firstValue(res, "code"))
		out.Name = asString(firstValue(res, "name"))
		out.Listcount = asString(firstValue(res, "listCount", "listcount"))
		out.Regday = asString(firstValue(res, "regDay", "regday"))
		out.State = asString(firstValue(res, "state"))
		out.Marketcode = asString(firstValue(res, "marketCode", "marketcode"))
		out.Marketname = asString(firstValue(res, "marketName", "marketname"))
		out.Upname = asString(firstValue(res, "upName", "upname"))
	}
	out.Code = normalizeSymbolCode(out.Code)
	if out.Code == "" {
		return nil, broker.ErrInstrumentNotFound
	}
	return out, nil
}

// InquireInstrumentInfo fetches ka10100.
func (c *Client) InquireInstrumentInfo(
	ctx context.Context,
	symbol string,
) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10100Response, error) {
	return c.InquireInstrumentInfoByRequest(ctx, kiwoomspecs.KiwoomApiDostkStkinfoKa10100Request{
		StkCd: symbol,
	})
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}
