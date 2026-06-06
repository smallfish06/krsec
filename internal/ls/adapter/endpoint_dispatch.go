package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	lsspecs "github.com/smallfish06/krsec/pkg/ls/specs"
)

// CallEndpoint executes a documented LS REST endpoint by path and tr_cd.
func (a *Adapter) CallEndpoint(ctx context.Context, method, path, trCD string, request interface{}) (interface{}, error) {
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

func validateDocumentedRequestFields(spec lsspecs.LSEndpointSpec, payload map[string]interface{}) error {
	for _, field := range spec.RequestFields {
		if !field.Required {
			continue
		}
		code := strings.TrimSpace(field.Code)
		if code == "" {
			continue
		}
		if !payloadHasField(payload, code) {
			return fmt.Errorf("%w: missing required field %s", broker.ErrInvalidOrderRequest, code)
		}
	}
	return nil
}

func requestPayloadMap(request interface{}) (map[string]interface{}, error) {
	switch t := request.(type) {
	case nil:
		return map[string]interface{}{}, nil
	case map[string]interface{}:
		return clonePayloadMap(t), nil
	case map[string]string:
		out := make(map[string]interface{}, len(t))
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
			return map[string]interface{}{}, nil
		}
		var out map[string]interface{}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("decode request payload: %w", err)
		}
		if out == nil {
			out = map[string]interface{}{}
		}
		return out, nil
	}
}

func clonePayloadMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func payloadHasField(value interface{}, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	switch t := value.(type) {
	case map[string]interface{}:
		for k, v := range t {
			if strings.EqualFold(strings.TrimSpace(k), key) {
				return payloadValuePresent(v)
			}
			if payloadHasField(v, key) {
				return true
			}
		}
	case map[string]string:
		for k, v := range t {
			if strings.EqualFold(strings.TrimSpace(k), key) {
				return strings.TrimSpace(v) != ""
			}
		}
	case []interface{}:
		for _, item := range t {
			if payloadHasField(item, key) {
				return true
			}
		}
	}
	return false
}

func payloadValuePresent(value interface{}) bool {
	switch t := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(t) != ""
	case map[string]interface{}:
		return t != nil
	case map[string]string:
		return t != nil
	case []interface{}:
		return t != nil
	default:
		return true
	}
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
