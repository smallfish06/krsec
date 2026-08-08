// Package specs holds the generated documented KIS endpoint specs
// and their shared runtime helpers.
package specs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// DocumentedEndpointResponse is the typed response contract for documented KIS endpoints.
type DocumentedEndpointResponse interface {
	IsSuccess() bool
	GetMsgCode() string
	GetMsg1() string
}

// DocumentedResponseBase holds common KIS response status fields.
type DocumentedResponseBase struct {
	RtCD  string `json:"rt_cd,omitempty"`
	MsgCD string `json:"msg_cd,omitempty"`
	Msg1  string `json:"msg1,omitempty"`
}

// IsSuccess reports whether the KIS response status code indicates success.
func (b *DocumentedResponseBase) IsSuccess() bool {
	rt := strings.TrimSpace(b.RtCD)
	// Some endpoints (e.g. hashkey) do not return rt_cd.
	if rt == "" {
		return true
	}
	return rt == "0"
}

// GetMsgCode returns the trimmed KIS message code (msg_cd).
func (b *DocumentedResponseBase) GetMsgCode() string {
	return strings.TrimSpace(b.MsgCD)
}

// GetMsg1 returns the trimmed KIS message text (msg1).
func (b *DocumentedResponseBase) GetMsg1() string {
	return strings.TrimSpace(b.Msg1)
}

// NewDocumentedEndpointResponse returns a typed response object for the endpoint path.
func NewDocumentedEndpointResponse(path string) DocumentedEndpointResponse {
	if f, ok := documentedEndpointResponseFactories[strings.TrimSpace(path)]; ok {
		return f()
	}
	return nil
}

// NewDocumentedEndpointRequest returns a typed request object for the endpoint path.
func NewDocumentedEndpointRequest(path string) any {
	if f, ok := documentedEndpointRequestFactories[strings.TrimSpace(path)]; ok {
		return f()
	}
	return nil
}

// DocumentedEndpointResponseFactoryCount returns the number of typed documented endpoint responses.
func DocumentedEndpointResponseFactoryCount() int {
	return len(documentedEndpointResponseFactories)
}

// DocumentedEndpointRequestFactoryCount returns the number of typed documented endpoint requests.
func DocumentedEndpointRequestFactoryCount() int {
	return len(documentedEndpointRequestFactories)
}

// DocumentedSlice accepts both object and array payloads and normalizes them to a typed slice.
// KIS documented payloads occasionally disagree with runtime payload container shape.
type DocumentedSlice[T any] []T

// UnmarshalJSON accepts either a JSON array or a single object and
// normalizes both to a slice.
func (s *DocumentedSlice[T]) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = nil
		return nil
	}

	switch data[0] {
	case '[':
		var out []T
		if err := json.Unmarshal(data, &out); err != nil {
			return err
		}
		*s = DocumentedSlice[T](out)
		return nil
	case '{':
		var one T
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		*s = DocumentedSlice[T]{one}
		return nil
	default:
		return fmt.Errorf("expected array/object JSON container")
	}
}

// MarshalJSON always emits the canonical JSON array form.
func (s DocumentedSlice[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal([]T(s))
}
