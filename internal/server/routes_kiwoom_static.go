package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/kiwoom"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

func (s *Server) registerKiwoomStaticProxyRoutes() {
	keys := make([]string, 0, len(kiwoomspecs.DocumentedKiwoomEndpointSpecs))
	for key := range kiwoomspecs.DocumentedKiwoomEndpointSpecs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		spec := kiwoomspecs.DocumentedKiwoomEndpointSpecs[key]
		proxyPath := toKiwoomStaticProxyPath(spec.Path, spec.APIID)
		if proxyPath == "" {
			continue
		}

		desc := fmt.Sprintf(
			"Static documented Kiwoom proxy route for %s %s (api_id=%s).",
			strings.ToUpper(strings.TrimSpace(spec.Method)),
			spec.Path,
			spec.APIID,
		)
		summary := "Call Kiwoom static endpoint " + proxyPath

		options := []fuego.RouteOption{
			fuego.OptionTags("Kiwoom"),
			fuego.OptionSummary(summary),
			fuego.OptionDescription(desc),
			fuego.OptionQuery("account_id", "Optional account selector when multiple Kiwoom accounts exist."),
		}

		if reqType := kiwoomspecs.NewDocumentedEndpointRequest(spec.Path, spec.APIID); reqType != nil {
			options = append(options, fuego.OptionRequestBody(fuego.RequestBody{
				Type:         reqType,
				ContentTypes: []string{"application/json"},
			}))
		}
		if respType := kiwoomspecs.NewDocumentedEndpointResponse(spec.Path, spec.APIID); respType != nil {
			options = append(options, fuego.OptionAddResponse(http.StatusOK, "OK", fuego.Response{
				Type:         respType,
				ContentTypes: []string{"application/json"},
			}))
		}

		fuego.Post(s.router, proxyPath, s.handleKiwoomProxyStatic(spec.Path, spec.APIID), options...)
	}
}

func toKiwoomStaticProxyPath(path, apiID string) string {
	p := strings.TrimSpace(path)
	id := strings.ToLower(strings.TrimSpace(apiID))
	if p == "" || id == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, kiwoom.PathPrefixAPISlash) {
		return ""
	}

	trimmed := strings.TrimPrefix(p, kiwoom.PathPrefixAPI)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	return "/kiwoom" + trimmed + "/" + id
}
