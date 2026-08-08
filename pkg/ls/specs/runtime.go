// Package specs holds the generated documented LS endpoint specs
// and their shared runtime helpers.
package specs

import "strings"

const (
	// ProtocolREST identifies LS REST APIs in the official OpenAPI guide.
	ProtocolREST = "REST"
	// ProtocolWebSocket identifies LS realtime APIs in the official OpenAPI guide.
	ProtocolWebSocket = "WEBSOCKET"
)

// LSFieldSpec describes one request/response/header field from the LS docs.
type LSFieldSpec struct {
	Code        string
	Name        string
	Type        string
	Length      string
	Order       string
	Required    bool
	Description string
}

// LSEndpointSpec defines one LS path/tr_cd specification from official docs.
type LSEndpointSpec struct {
	Path              string
	TRCode            string
	Method            string
	Protocol          string
	GroupName         string
	APIName           string
	TRName            string
	TransactionPerSec string
	RequestHeaders    []LSFieldSpec
	RequestFields     []LSFieldSpec
	ResponseHeaders   []LSFieldSpec
	ResponseFields    []LSFieldSpec
}

func documentedEndpointKey(path, trCD string) string {
	return normalizePath(path) + "|" + strings.ToLower(strings.TrimSpace(trCD))
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// LookupDocumentedEndpointSpec finds one generated LS endpoint spec by path/tr_cd.
func LookupDocumentedEndpointSpec(path, trCD string) (LSEndpointSpec, bool) {
	spec, ok := DocumentedLSEndpointSpecs[documentedEndpointKey(path, trCD)]
	return spec, ok
}

// DocumentedEndpointSpecCount returns the number of generated LS endpoint specs.
func DocumentedEndpointSpecCount() int {
	return len(DocumentedLSEndpointSpecs)
}

// DocumentedRESTEndpointSpecCount returns the number of generated LS REST endpoint specs.
func DocumentedRESTEndpointSpecCount() int {
	var count int
	for _, spec := range DocumentedLSEndpointSpecs {
		if strings.EqualFold(strings.TrimSpace(spec.Protocol), ProtocolREST) {
			count++
		}
	}
	return count
}

// DocumentedWebSocketEndpointSpecCount returns the number of generated LS WebSocket endpoint specs.
func DocumentedWebSocketEndpointSpecCount() int {
	var count int
	for _, spec := range DocumentedLSEndpointSpecs {
		if strings.EqualFold(strings.TrimSpace(spec.Protocol), ProtocolWebSocket) {
			count++
		}
	}
	return count
}
