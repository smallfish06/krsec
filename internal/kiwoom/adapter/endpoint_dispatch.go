package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/smallfish06/krsec/internal/endpointpath"
	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

type endpointDispatchFunc func(ctx context.Context, request any) (any, error)

type endpointRouteKey struct {
	path  string
	apiID string
}

type endpointDispatcher struct {
	adapter *Adapter
	routes  map[endpointRouteKey]endpointRoute
}

type endpointRoute struct {
	methods       map[string]struct{}
	defaultMethod string
	fn            endpointDispatchFunc
}

type overrideRouteDef struct {
	path  string
	apiID string
	fn    endpointDispatchFunc
}

func newEndpointRoute(methods []string, fn endpointDispatchFunc) endpointRoute {
	m := make(map[string]struct{}, len(methods))
	defaultMethod := ""
	for _, method := range methods {
		n := strings.ToUpper(strings.TrimSpace(method))
		if n == "" {
			continue
		}
		if defaultMethod == "" {
			defaultMethod = n
		}
		m[n] = struct{}{}
	}
	return endpointRoute{methods: m, defaultMethod: defaultMethod, fn: fn}
}

func (r endpointRoute) allows(method string) bool {
	_, ok := r.methods[strings.ToUpper(strings.TrimSpace(method))]
	return ok
}

func newEndpointDispatcher(adapter *Adapter) *endpointDispatcher {
	d := &endpointDispatcher{
		adapter: adapter,
		routes:  make(map[endpointRouteKey]endpointRoute, kiwoomspecs.DocumentedEndpointSpecCount()),
	}
	d.registerDocumentedRoutes()
	d.registerCoreOverrides()
	return d
}

func (d *endpointDispatcher) registerDocumentedRoutes() {
	for _, spec := range kiwoomspecs.DocumentedKiwoomEndpointSpecs {
		path := normalizeEndpointPath(spec.Path)
		apiID := normalizeEndpointAPIID(spec.APIID)
		if path == "" || apiID == "" {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(spec.Method))
		if method == "" {
			method = http.MethodPost
		}
		routeSpec := spec
		d.routes[endpointRouteKey{path: path, apiID: apiID}] = newEndpointRoute(
			[]string{method},
			d.dispatchDocumentedEndpoint(path, apiID, routeSpec),
		)
	}
}

func (d *endpointDispatcher) registerCoreOverrides() {
	postMethods := []string{http.MethodPost}
	defs := []overrideRouteDef{
		{
			path:  kiwoom.PathStockInfo,
			apiID: "ka10001",
			fn: newRequestDispatchPrepared[kiwoomspecs.KiwoomApiDostkStkinfoKa10001Request, *kiwoomspecs.KiwoomApiDostkStkinfoKa10001Response](
				d,
				func(req *kiwoomspecs.KiwoomApiDostkStkinfoKa10001Request) error {
					if strings.TrimSpace(req.StkCd) == "" {
						return broker.ErrInvalidSymbol
					}
					return nil
				},
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkStkinfoKa10001Request) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10001Response, error) {
					return client.InquirePriceByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathStockInfo,
			apiID: "ka10003",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkStkinfoKa10003Request, *kiwoomspecs.KiwoomApiDostkStkinfoKa10003Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkStkinfoKa10003Request) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10003Response, error) {
					return client.InquireExecutionInfo(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathStockInfo,
			apiID: "ka10100",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkStkinfoKa10100Request, *kiwoomspecs.KiwoomApiDostkStkinfoKa10100Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkStkinfoKa10100Request) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10100Response, error) {
					return client.InquireInstrumentInfoByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathStockInfo,
			apiID: "ka10059",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkStkinfoKa10059Request, *kiwoomspecs.KiwoomApiDostkStkinfoKa10059Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkStkinfoKa10059Request) (*kiwoomspecs.KiwoomApiDostkStkinfoKa10059Response, error) {
					return client.InquireInvestorByStock(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathMarketCond,
			apiID: "ka10004",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkMrkcondKa10004Request, *kiwoomspecs.KiwoomApiDostkMrkcondKa10004Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkMrkcondKa10004Request) (*kiwoomspecs.KiwoomApiDostkMrkcondKa10004Response, error) {
					return client.InquireOrderBook(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathRankingInfo,
			apiID: "ka10030",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkRkinfoKa10030Request, *kiwoomspecs.KiwoomApiDostkRkinfoKa10030Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkRkinfoKa10030Request) (*kiwoomspecs.KiwoomApiDostkRkinfoKa10030Response, error) {
					return client.InquireVolumeRank(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathRankingInfo,
			apiID: "ka10027",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkRkinfoKa10027Request, *kiwoomspecs.KiwoomApiDostkRkinfoKa10027Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkRkinfoKa10027Request) (*kiwoomspecs.KiwoomApiDostkRkinfoKa10027Response, error) {
					return client.InquireChangeRateRank(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathSector,
			apiID: "ka20001",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkSectKa20001Request, *kiwoomspecs.KiwoomApiDostkSectKa20001Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkSectKa20001Request) (*kiwoomspecs.KiwoomApiDostkSectKa20001Response, error) {
					return client.InquireSectorCurrent(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathSector,
			apiID: "ka20002",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkSectKa20002Request, *kiwoomspecs.KiwoomApiDostkSectKa20002Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkSectKa20002Request) (*kiwoomspecs.KiwoomApiDostkSectKa20002Response, error) {
					return client.InquireSectorByPrice(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "kt00005",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKt00005Request, *kiwoomspecs.KiwoomApiDostkAcntKt00005Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKt00005Request) (*kiwoomspecs.KiwoomApiDostkAcntKt00005Response, error) {
					return client.InquireBalanceByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "kt00018",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKt00018Request, *kiwoomspecs.KiwoomApiDostkAcntKt00018Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKt00018Request) (*kiwoomspecs.KiwoomApiDostkAcntKt00018Response, error) {
					return client.InquirePositionsByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "ka10075",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKa10075Request, *kiwoomspecs.KiwoomApiDostkAcntKa10075Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKa10075Request) (*kiwoomspecs.KiwoomApiDostkAcntKa10075Response, error) {
					return client.InquireUnsettledOrdersByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "ka10076",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKa10076Request, *kiwoomspecs.KiwoomApiDostkAcntKa10076Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKa10076Request) (*kiwoomspecs.KiwoomApiDostkAcntKa10076Response, error) {
					return client.InquireOrderExecutionsByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "kt00007",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKt00007Request, *kiwoomspecs.KiwoomApiDostkAcntKt00007Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKt00007Request) (*kiwoomspecs.KiwoomApiDostkAcntKt00007Response, error) {
					return client.InquireOrderExecutionDetail(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "kt00009",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKt00009Request, *kiwoomspecs.KiwoomApiDostkAcntKt00009Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKt00009Request) (*kiwoomspecs.KiwoomApiDostkAcntKt00009Response, error) {
					return client.InquireOrderExecutionStatus(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathAccount,
			apiID: "kt00010",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkAcntKt00010Request, *kiwoomspecs.KiwoomApiDostkAcntKt00010Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkAcntKt00010Request) (*kiwoomspecs.KiwoomApiDostkAcntKt00010Response, error) {
					return client.InquireOrderableWithdrawable(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathChart,
			apiID: "ka10079",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkChartKa10079Request, *kiwoomspecs.KiwoomApiDostkChartKa10079Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkChartKa10079Request) (*kiwoomspecs.KiwoomApiDostkChartKa10079Response, error) {
					return client.InquireTickChartByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathChart,
			apiID: "ka10060",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkChartKa10060Request, *kiwoomspecs.KiwoomApiDostkChartKa10060Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkChartKa10060Request) (*kiwoomspecs.KiwoomApiDostkChartKa10060Response, error) {
					return client.InquireInvestorByStockChart(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathChart,
			apiID: "ka10081",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkChartKa10081Request, *kiwoomspecs.KiwoomApiDostkChartKa10081Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkChartKa10081Request) (*kiwoomspecs.KiwoomApiDostkChartKa10081Response, error) {
					return client.InquireDailyPriceByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathChart,
			apiID: "ka10082",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkChartKa10082Request, *kiwoomspecs.KiwoomApiDostkChartKa10082Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkChartKa10082Request) (*kiwoomspecs.KiwoomApiDostkChartKa10082Response, error) {
					return client.InquireWeeklyPriceByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathChart,
			apiID: "ka10083",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkChartKa10083Request, *kiwoomspecs.KiwoomApiDostkChartKa10083Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkChartKa10083Request) (*kiwoomspecs.KiwoomApiDostkChartKa10083Response, error) {
					return client.InquireMonthlyPriceByRequest(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathOrder,
			apiID: "kt10000",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkOrdrKt10000Request, *kiwoomspecs.KiwoomApiDostkOrdrKt10000Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkOrdrKt10000Request) (*kiwoomspecs.KiwoomApiDostkOrdrKt10000Response, error) {
					return client.PlaceBuyOrder(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathOrder,
			apiID: "kt10001",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkOrdrKt10001Request, *kiwoomspecs.KiwoomApiDostkOrdrKt10000Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkOrdrKt10001Request) (*kiwoomspecs.KiwoomApiDostkOrdrKt10000Response, error) {
					return client.PlaceSellOrder(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathOrder,
			apiID: "kt10002",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkOrdrKt10002Request, *kiwoomspecs.KiwoomApiDostkOrdrKt10002Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkOrdrKt10002Request) (*kiwoomspecs.KiwoomApiDostkOrdrKt10002Response, error) {
					return client.ModifyStockOrder(callCtx, req)
				},
			),
		},
		{
			path:  kiwoom.PathOrder,
			apiID: "kt10003",
			fn: newRequestDispatch[kiwoomspecs.KiwoomApiDostkOrdrKt10003Request, *kiwoomspecs.KiwoomApiDostkOrdrKt10003Response](
				d,
				func(client *kiwoom.Client, callCtx context.Context, req kiwoomspecs.KiwoomApiDostkOrdrKt10003Request) (*kiwoomspecs.KiwoomApiDostkOrdrKt10003Response, error) {
					return client.CancelStockOrder(callCtx, req)
				},
			),
		},
	}
	for _, def := range defs {
		d.registerOverrideRoute(def.path, def.apiID, postMethods, def.fn)
	}
}

func (d *endpointDispatcher) registerOverrideRoute(path, apiID string, methods []string, fn endpointDispatchFunc) {
	d.routes[endpointRouteKey{
		path:  normalizeEndpointPath(path),
		apiID: normalizeEndpointAPIID(apiID),
	}] = newEndpointRoute(methods, fn)
}

func (d *endpointDispatcher) dispatchDocumentedEndpoint(
	path string,
	apiID string,
	spec kiwoomspecs.KiwoomEndpointSpec,
) endpointDispatchFunc {
	return func(ctx context.Context, request any) (any, error) {
		payload, err := requestPayloadMap(request)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", broker.ErrInvalidOrderRequest, err)
		}
		applyDocumentedDefaults(apiID, payload)
		for _, required := range spec.RequiredFields {
			if payloadFieldEmpty(payload, required) {
				return nil, fmt.Errorf("%w: missing required field %s", broker.ErrInvalidOrderRequest, strings.ToUpper(strings.TrimSpace(required)))
			}
		}

		client, err := d.client()
		if err != nil {
			return nil, err
		}

		req, err := buildDocumentedEndpointRequest(path, apiID, payload)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", broker.ErrInvalidOrderRequest, err)
		}
		resp, err := client.CallDocumentedEndpoint(ctx, apiID, path, req)
		return resp, err
	}
}

func applyDocumentedDefaults(apiID string, payload map[string]any) {
	switch strings.ToLower(strings.TrimSpace(apiID)) {
	case "ka50079", "ka50080":
		if payloadFieldEmpty(payload, "tic_scope") {
			payload["tic_scope"] = "1"
		}
	}
}

func payloadFieldEmpty(payload map[string]any, key string) bool {
	if payload == nil {
		return true
	}
	v, ok := payload[strings.ToLower(strings.TrimSpace(key))]
	if !ok || v == nil {
		return true
	}
	return strings.TrimSpace(fmt.Sprint(v)) == ""
}

// CallEndpoint dispatches a Kiwoom endpoint path/api_id to implemented client methods.
func (a *Adapter) CallEndpoint(
	ctx context.Context,
	method string,
	path string,
	apiID string,
	request any,
) (any, error) {
	dispatcher := a.dispatcher
	if dispatcher == nil {
		dispatcher = newEndpointDispatcher(a)
	}
	return dispatcher.callEndpoint(ctx, method, path, apiID, request)
}

func (d *endpointDispatcher) callEndpoint(
	ctx context.Context,
	method string,
	path string,
	apiID string,
	request any,
) (any, error) {
	m := strings.ToUpper(strings.TrimSpace(method))

	normalizedPath := normalizeEndpointPath(path)
	normalizedAPIID := normalizeEndpointAPIID(apiID)

	if normalizedAPIID == "" {
		return nil, fmt.Errorf("%w: api_id is required", broker.ErrInvalidOrderRequest)
	}

	route, ok := d.routes[endpointRouteKey{path: normalizedPath, apiID: normalizedAPIID}]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported Kiwoom endpoint path/api_id %s/%s", broker.ErrInvalidOrderRequest, normalizedPath, normalizedAPIID)
	}
	if m == "" {
		m = route.defaultMethod
	}
	if !route.allows(m) {
		return nil, fmt.Errorf("%w: unsupported method %s", broker.ErrInvalidOrderRequest, m)
	}

	normalizedRequest, err := normalizeEndpointRequest(normalizedPath, normalizedAPIID, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", broker.ErrInvalidOrderRequest, err)
	}

	return route.fn(ctx, normalizedRequest)
}

func normalizeEndpointPath(path string) string {
	return endpointpath.Normalize(path, kiwoom.PathPrefixAPI, kiwoom.PathPrefixAPISlash)
}

func normalizeEndpointAPIID(apiID string) string {
	return strings.ToLower(strings.TrimSpace(apiID))
}

func normalizeEndpointRequest(path, apiID string, request any) (any, error) {
	switch request.(type) {
	case nil, map[string]any:
		payload, err := requestPayloadMap(request)
		if err != nil {
			return nil, err
		}
		return buildDocumentedEndpointRequest(path, apiID, payload)
	default:
		return request, nil
	}
}

func requestPayloadMap(request any) (map[string]any, error) {
	switch t := request.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return normalizePayloadMap(t), nil
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal request payload: %w", err)
		}
		if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return map[string]any{}, nil
		}
		out := make(map[string]any)
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("decode request payload: %w", err)
		}
		return normalizePayloadMap(out), nil
	}
}

func buildDocumentedEndpointRequest(path, apiID string, payload map[string]any) (any, error) {
	req := kiwoomspecs.NewDocumentedEndpointRequest(strings.TrimSpace(path), strings.TrimSpace(apiID))
	if req == nil {
		return clonePayloadMap(payload), nil
	}
	if payload == nil {
		return req, nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal documented request payload: %w", err)
	}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("decode documented request payload: %w", err)
	}
	return req, nil
}

func clonePayloadMap(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	return maps.Clone(payload)
}

func normalizePayloadMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func newRequestDispatch[TReq any, TResp any](
	d *endpointDispatcher,
	call func(client *kiwoom.Client, ctx context.Context, req TReq) (TResp, error),
) endpointDispatchFunc {
	return newRequestDispatchPrepared(d, nil, call)
}

func newRequestDispatchPrepared[TReq any, TResp any](
	d *endpointDispatcher,
	prepare func(*TReq) error,
	call func(client *kiwoom.Client, ctx context.Context, req TReq) (TResp, error),
) endpointDispatchFunc {
	return func(ctx context.Context, request any) (any, error) {
		return dispatchWithRequestPrepared(d, ctx, request, prepare, call)
	}
}

func dispatchWithRequestPrepared[TReq any, TResp any](
	d *endpointDispatcher,
	ctx context.Context,
	request any,
	prepare func(*TReq) error,
	call func(client *kiwoom.Client, ctx context.Context, req TReq) (TResp, error),
) (any, error) {
	req, err := decodeDispatchRequestAs[TReq](request)
	if err != nil {
		return nil, err
	}
	if prepare != nil {
		if err := prepare(&req); err != nil {
			return nil, err
		}
	}

	client, err := d.client()
	if err != nil {
		return nil, err
	}

	resp, err := call(client, ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func decodeDispatchRequestAs[T any](request any) (T, error) {
	var req T
	if err := decodeDispatchRequest(request, &req); err != nil {
		var zero T
		return zero, err
	}
	return req, nil
}

func decodeDispatchRequest(request any, out any) error {
	if out == nil {
		return fmt.Errorf("%w: request target is nil", broker.ErrInvalidOrderRequest)
	}
	if request == nil {
		return nil
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("%w: marshal request: %v", broker.ErrInvalidOrderRequest, err)
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: decode request: %v", broker.ErrInvalidOrderRequest, err)
	}
	return nil
}

func (d *endpointDispatcher) client() (*kiwoom.Client, error) {
	if d == nil || d.adapter == nil || d.adapter.client == nil {
		return nil, fmt.Errorf("%w: kiwoom client is not initialized", broker.ErrInvalidOrderRequest)
	}
	return d.adapter.client, nil
}
