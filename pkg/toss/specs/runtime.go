// Package specs holds the generated documented Toss endpoint specs
// and their shared runtime helpers.
package specs

import "strings"

// TossEndpointSpec defines one operation from the Toss Open API document.
type TossEndpointSpec struct {
	Path            string
	Method          string
	OperationID     string
	Tag             string
	Summary         string
	RateLimitGroup  string
	AccountRequired bool
}

func documentedEndpointKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizePath(path)
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

// LookupDocumentedEndpointSpec finds one generated Toss operation spec by method/path.
func LookupDocumentedEndpointSpec(method, path string) (TossEndpointSpec, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizePath(path)
	spec, ok := DocumentedTossEndpointSpecs[documentedEndpointKey(method, path)]
	if ok {
		return spec, true
	}
	for _, spec := range DocumentedTossEndpointSpecs {
		if spec.Method != method {
			continue
		}
		if pathMatchesTemplate(spec.Path, path) {
			return spec, true
		}
	}
	return TossEndpointSpec{}, false
}

// DefaultMethodForPath returns the only documented method for a path.
func DefaultMethodForPath(path string) (string, bool) {
	path = normalizePath(path)
	if path == "" {
		return "", false
	}
	var method string
	for _, spec := range DocumentedTossEndpointSpecs {
		if normalizePath(spec.Path) != path {
			continue
		}
		if method != "" && method != spec.Method {
			return "", false
		}
		method = spec.Method
	}
	return method, method != ""
}

// DocumentedEndpointSpecCount returns the number of generated Toss operations.
func DocumentedEndpointSpecCount() int {
	return len(DocumentedTossEndpointSpecs)
}

func pathMatchesTemplate(template, path string) bool {
	template = strings.Trim(normalizePath(template), "/")
	path = strings.Trim(normalizePath(path), "/")
	if template == path {
		return true
	}
	tParts := strings.Split(template, "/")
	pParts := strings.Split(path, "/")
	if len(tParts) != len(pParts) {
		return false
	}
	for i := range tParts {
		t := strings.TrimSpace(tParts[i])
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			if strings.TrimSpace(pParts[i]) == "" {
				return false
			}
			continue
		}
		if t != pParts[i] {
			return false
		}
	}
	return true
}
