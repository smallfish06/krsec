package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	lsspecs "github.com/smallfish06/krsec/pkg/ls/specs"
)

// CallEndpoint executes a documented LS REST endpoint by path and tr_cd.
func (a *Adapter) CallEndpoint(ctx context.Context, method, path, trCD string, request any) (any, error) {
	path = normalizeEndpointPath(path)
	trCD = strings.TrimSpace(trCD)
	if path == "" {
		return nil, fmt.Errorf("%w: path is required", broker.ErrInvalidOrderRequest)
	}
	if trCD == "" {
		return nil, fmt.Errorf("%w: tr_cd is required", broker.ErrInvalidOrderRequest)
	}
	if strings.HasPrefix(path, "/oauth2/") {
		return nil, fmt.Errorf("%w: LS OAuth endpoints are handled by Authenticate", broker.ErrInvalidOrderRequest)
	}

	spec, ok := lsspecs.LookupDocumentedEndpointSpec(path, trCD)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported LS endpoint path/tr_cd %s/%s", broker.ErrInvalidOrderRequest, path, trCD)
	}
	if !strings.EqualFold(spec.Protocol, lsspecs.ProtocolREST) {
		return nil, fmt.Errorf("%w: LS tr_cd %s is %s; use ConnectRealtime", broker.ErrInvalidOrderRequest, trCD, spec.Protocol)
	}

	effectiveMethod := strings.ToUpper(strings.TrimSpace(method))
	if effectiveMethod == "" {
		effectiveMethod = strings.ToUpper(strings.TrimSpace(spec.Method))
	}
	if effectiveMethod == "" {
		effectiveMethod = http.MethodPost
	}
	if specMethod := strings.ToUpper(strings.TrimSpace(spec.Method)); specMethod != "" && effectiveMethod != specMethod {
		return nil, fmt.Errorf("%w: unsupported method %s for LS endpoint %s/%s", broker.ErrInvalidOrderRequest, effectiveMethod, path, trCD)
	}

	payload, err := requestPayloadMap(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", broker.ErrInvalidOrderRequest, err)
	}
	if err := validateDocumentedRequestFields(spec, payload); err != nil {
		return nil, err
	}
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("%w: LS client is not initialized", broker.ErrInvalidOrderRequest)
	}
	return a.client.CallEndpoint(ctx, effectiveMethod, path, trCD, payload)
}

func validateDocumentedRequestFields(spec lsspecs.LSEndpointSpec, payload map[string]any) error {
	for _, field := range spec.RequestFields {
		if !field.Required {
			continue
		}
		code := strings.TrimSpace(field.Code)
		if code == "" {
			continue
		}
		if isOptionalContinuationField(code) {
			continue
		}
		if !payloadHasField(payload, code) {
			return fmt.Errorf("%w: missing required field %s", broker.ErrInvalidOrderRequest, code)
		}
	}
	return nil
}

func isOptionalContinuationField(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "cts_date", "cts_info", "cts_seq", "cts_value", "sujung":
		return true
	default:
		return false
	}
}

func requestPayloadMap(request any) (map[string]any, error) {
	switch t := request.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return clonePayloadMap(t), nil
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = v
		}
		return out, nil
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal request payload: %w", err)
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 || bytes.Equal(data, []byte("null")) {
			return map[string]any{}, nil
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("decode request payload: %w", err)
		}
		if out == nil {
			out = map[string]any{}
		}
		return out, nil
	}
}

func clonePayloadMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func payloadHasField(value any, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	switch t := value.(type) {
	case map[string]any:
		for k, v := range t {
			if strings.EqualFold(strings.TrimSpace(k), key) {
				return v != nil
			}
			if payloadHasField(v, key) {
				return true
			}
		}
	case map[string]string:
		for k := range t {
			if strings.EqualFold(strings.TrimSpace(k), key) {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if payloadHasField(item, key) {
				return true
			}
		}
	}
	return false
}

func normalizeEndpointPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
