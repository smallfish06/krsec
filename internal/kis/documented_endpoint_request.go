package kis

import (
	"encoding/json"
	"fmt"
	"strings"

	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

// DocumentedRequestFields normalizes documented request structs/maps into endpoint fields.
func DocumentedRequestFields(v any) (map[string]string, error) {
	if v == nil {
		return map[string]string{}, nil
	}
	if fields, ok := v.(map[string]string); ok {
		out := make(map[string]string, len(fields))
		for k, val := range fields {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(val)
		}
		return out, nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal documented request: %w", err)
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decode documented request fields: %w", err)
	}
	if fields == nil {
		return map[string]string{}, nil
	}
	for k, val := range fields {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			delete(fields, k)
			continue
		}
		if trimmed != k {
			delete(fields, k)
			fields[trimmed] = strings.TrimSpace(val)
			continue
		}
		fields[k] = strings.TrimSpace(val)
	}
	return fields, nil
}

// NewDocumentedEndpointRequest returns a typed request object for the endpoint path.
func NewDocumentedEndpointRequest(path string) any {
	return kisspecs.NewDocumentedEndpointRequest(strings.TrimSpace(path))
}

// DocumentedEndpointRequestFactoryCount returns the number of typed documented endpoint requests.
func DocumentedEndpointRequestFactoryCount() int {
	return kisspecs.DocumentedEndpointRequestFactoryCount()
}
