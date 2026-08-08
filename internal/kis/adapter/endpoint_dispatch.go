package adapter

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/smallfish06/krsec/internal/endpointpath"
	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/pkg/broker"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

type endpointDispatchFunc func(ctx context.Context, method string, trID string, fields map[string]string) (any, error)

type endpointDispatcher struct {
	adapter *Adapter
	routes  map[string]endpointRoute
}

type endpointRoute struct {
	methods       map[string]struct{}
	defaultMethod string
	fn            endpointDispatchFunc
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
		routes:  make(map[string]endpointRoute, len(kisspecs.DocumentedKISEndpointSpecs)),
	}
	d.registerDocumentedKISRoutes()
	return d
}

func (d *endpointDispatcher) registerDocumentedKISRoutes() {
	for path, spec := range kisspecs.DocumentedKISEndpointSpecs {
		p := path
		method := strings.ToUpper(strings.TrimSpace(spec.Method))
		if method == "" {
			method = http.MethodGet
		}
		d.routes[p] = newEndpointRoute([]string{method}, d.dispatchDocumentedKISEndpoint(p, spec))
	}
}

func (d *endpointDispatcher) dispatchDocumentedKISEndpoint(path string, spec kisspecs.KISEndpointSpec) endpointDispatchFunc {
	return func(ctx context.Context, method string, trID string, fields map[string]string) (any, error) {
		for _, req := range spec.RequiredFields {
			k := strings.ToUpper(strings.TrimSpace(req))
			if k == "" {
				continue
			}
			if _, ok := fields[k]; !ok {
				return nil, fmt.Errorf("%w: missing required field %s", broker.ErrInvalidOrderRequest, k)
			}
		}

		if d.adapter == nil || d.adapter.client == nil {
			return nil, fmt.Errorf("%w: kis client is not initialized", broker.ErrInvalidOrderRequest)
		}

		effectiveTRID := strings.TrimSpace(trID)
		if effectiveTRID == "" {
			effectiveTRID = d.adapter.client.ResolveTRID(spec.RealTRID, spec.VirtualTRID)
		}

		resp := kisspecs.NewDocumentedEndpointResponse(strings.TrimSpace(path))
		if resp == nil {
			return nil, fmt.Errorf("%w: missing documented response type for path %s", broker.ErrInvalidOrderRequest, path)
		}
		if err := d.adapter.client.CallDocumentedEndpointInto(ctx, method, path, effectiveTRID, fields, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func (d *endpointDispatcher) callEndpoint(
	ctx context.Context,
	method string,
	path string,
	trID string,
	request any,
) (any, error) {
	m := strings.ToUpper(strings.TrimSpace(method))

	normalizedPath := normalizeEndpointPath(path)
	normalizedFields, err := kis.DocumentedRequestFields(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", broker.ErrInvalidOrderRequest, err)
	}
	normalizedFields = normalizeEndpointFields(normalizedFields)

	route, ok := d.routes[normalizedPath]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported KIS endpoint path %s", broker.ErrInvalidOrderRequest, normalizedPath)
	}
	if m == "" {
		m = route.defaultMethod
	}
	if !route.allows(m) {
		return nil, fmt.Errorf("%w: unsupported method %s", broker.ErrInvalidOrderRequest, m)
	}

	return route.fn(ctx, m, trID, normalizedFields)
}

// CallEndpoint dispatches a KIS endpoint path to documented endpoint specs.
func (a *Adapter) CallEndpoint(
	ctx context.Context,
	method string,
	path string,
	trID string,
	request any,
) (any, error) {
	dispatcher := a.dispatcher
	if dispatcher == nil {
		dispatcher = newEndpointDispatcher(a)
	}
	return dispatcher.callEndpoint(ctx, method, path, trID, request)
}

func normalizeEndpointPath(path string) string {
	return endpointpath.Normalize(path, kis.PathPrefixUAPI, kis.PathPrefixUAPISlash)
}

func normalizeEndpointFields(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.ToUpper(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}
