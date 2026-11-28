package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
)

const apiKeySecurityScheme = "apiKey"

// apiKeyServerOptions documents the gateway API key in the served OpenAPI
// spec so Swagger UI offers an Authorize button. Enforcement is installed
// separately by requireAPIKey.
func apiKeyServerOptions() []fuego.ServerOption {
	return []fuego.ServerOption{
		fuego.WithSecurity(openapi3.SecuritySchemes{
			apiKeySecurityScheme: &openapi3.SecuritySchemeRef{
				Value: openapi3.NewSecurityScheme().
					WithType("http").
					WithScheme("bearer").
					WithDescription("Gateway API key (server.api_key)."),
			},
		}),
		fuego.WithRouteOptions(
			fuego.OptionSecurity(openapi3.SecurityRequirement{apiKeySecurityScheme: {}}),
		),
	}
}

// requireAPIKey rejects requests that do not present key as a bearer token or
// X-API-Key header. Health checks and the API documentation stay open so
// orchestrator probes and Swagger UI work without credentials.
func requireAPIKey(key string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(key))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			// Comparing digests keeps the comparison constant-time regardless
			// of the presented key's length.
			got := sha256.Sum256([]byte(presentedAPIKey(r)))
			if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="krsec"`)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(Response{Error: "missing or invalid API key"})
		})
	}
}

func presentedAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(token)
	}
	return ""
}

func isPublicPath(path string) bool {
	return path == "/health" || path == "/swagger" || strings.HasPrefix(path, "/swagger/")
}

// isLoopbackAddr reports whether a host:port listen address is reachable only
// from the local machine.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
