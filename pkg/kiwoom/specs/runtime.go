// Package specs holds the generated documented Kiwoom endpoint specs
// and their shared runtime helpers.
package specs

import "strings"

func documentedEndpointKey(path, apiID string) string {
	return strings.TrimSpace(path) + "|" + strings.ToLower(strings.TrimSpace(apiID))
}

// LookupDocumentedEndpointSpec finds one generated Kiwoom endpoint spec by path/api_id.
func LookupDocumentedEndpointSpec(path, apiID string) (KiwoomEndpointSpec, bool) {
	spec, ok := DocumentedKiwoomEndpointSpecs[documentedEndpointKey(path, apiID)]
	return spec, ok
}

// DocumentedEndpointRequiredFields returns required request field codes for path/api_id.
func DocumentedEndpointRequiredFields(path, apiID string) []string {
	spec, ok := LookupDocumentedEndpointSpec(path, apiID)
	if !ok {
		return nil
	}
	out := make([]string, len(spec.RequiredFields))
	copy(out, spec.RequiredFields)
	return out
}

// NewDocumentedEndpointRequest returns a typed request object for path/api_id.
func NewDocumentedEndpointRequest(path, apiID string) any {
	if f, ok := documentedEndpointRequestFactories[documentedEndpointKey(path, apiID)]; ok {
		return f()
	}
	return nil
}

// NewDocumentedEndpointResponse returns a typed response object for path/api_id.
func NewDocumentedEndpointResponse(path, apiID string) any {
	if f, ok := documentedEndpointResponseFactories[documentedEndpointKey(path, apiID)]; ok {
		return f()
	}
	return nil
}

// DocumentedEndpointSpecCount returns number of generated Kiwoom endpoint specs.
func DocumentedEndpointSpecCount() int {
	return len(DocumentedKiwoomEndpointSpecs)
}

// DocumentedEndpointRequestFactoryCount returns number of generated request factories.
func DocumentedEndpointRequestFactoryCount() int {
	return len(documentedEndpointRequestFactories)
}

// DocumentedEndpointResponseFactoryCount returns number of generated response factories.
func DocumentedEndpointResponseFactoryCount() int {
	return len(documentedEndpointResponseFactories)
}
