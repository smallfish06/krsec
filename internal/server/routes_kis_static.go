package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/kis"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

func (s *Server) registerKISStaticProxyRoutes() {
	uapiPaths := make([]string, 0, len(kisspecs.DocumentedKISEndpointSpecs))
	for p := range kisspecs.DocumentedKISEndpointSpecs {
		uapiPaths = append(uapiPaths, p)
	}
	sort.Strings(uapiPaths)

	for _, uapiPath := range uapiPaths {
		spec := kisspecs.DocumentedKISEndpointSpecs[uapiPath]
		proxyPath := toKISStaticProxyPath(uapiPath)
		if proxyPath == "" {
			continue
		}

		desc := fmt.Sprintf("Static documented KIS proxy route for %s %s.", strings.ToUpper(strings.TrimSpace(spec.Method)), uapiPath)
		summary := "Call KIS static endpoint " + proxyPath

		options := []fuego.RouteOption{
			fuego.OptionTags("KIS"),
			fuego.OptionSummary(summary),
			fuego.OptionDescription(desc),
			fuego.OptionQuery("account_id", "Optional account selector when multiple KIS accounts exist."),
		}

		if reqType := kisspecs.NewDocumentedEndpointRequest(uapiPath); reqType != nil {
			options = append(options, fuego.OptionRequestBody(fuego.RequestBody{
				Type:         reqType,
				ContentTypes: []string{"application/json"},
			}))
		}
		if respType := kisspecs.NewDocumentedEndpointResponse(uapiPath); respType != nil {
			options = append(options, fuego.OptionAddResponse(http.StatusOK, "OK", fuego.Response{
				Type:         respType,
				ContentTypes: []string{"application/json"},
			}))
		}

		fuego.Post(s.router, proxyPath, s.handleKISProxyStatic(uapiPath), options...)
	}
}

func toKISStaticProxyPath(uapiPath string) string {
	p := strings.TrimSpace(uapiPath)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, kis.PathPrefixUAPISlash) {
		return ""
	}
	trimmed := strings.TrimPrefix(p, kis.PathPrefixUAPI)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	return "/kis" + trimmed
}
