package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultPortalURL = "https://apiportal.koreainvestment.com/apiservice-apiservice"
	defaultDetailURL = "https://apiportal.koreainvestment.com/api/apis/public/detail"

	defaultSnapshotPath = "pkg/kis/specs/documented_endpoints.json"
	defaultSpecsOutPath = "pkg/kis/specs/documented_specs_generated.go"
	defaultTypesOutPath = "pkg/kis/specs/documented_endpoint_types_generated.go"
)

type snapshot struct {
	Endpoints []endpointSnapshot `json:"endpoints"`
}

type endpointSnapshot struct {
	Path           string         `json:"path"`
	Method         string         `json:"method"`
	RealTRID       string         `json:"real_trid"`
	VirtualTRID    string         `json:"virtual_trid"`
	RequiredFields []string       `json:"required_fields,omitempty"`
	ResponseProps  []snapshotProp `json:"response_props,omitempty"`
}

type snapshotProp struct {
	Code  string `json:"code"`
	Type  string `json:"type,omitempty"`
	Order string `json:"order,omitempty"`
}

type detailResponse struct {
	AccessURL    string       `json:"accessUrl"`
	HTTPMethod   string       `json:"httpMethod"`
	RealTRID     string       `json:"realTrId"`
	VirtualTRID  string       `json:"virtualTrId"`
	APIPropertys []detailProp `json:"apiPropertys"`
}

type detailProp struct {
	BodyType      string `json:"bodyType"`
	PropertyCd    string `json:"propertyCd"`
	PropertyType  string `json:"propertyType"`
	PropertyOrder string `json:"propertyOrder"`
	RequireYn     string `json:"requireYn"`
}

type endpointModel struct {
	Path           string
	TypeName       string
	RequestType    string
	RequiredFields []string
	Top            []responseNode
}

type responseNode struct {
	Prop     snapshotProp
	Children []snapshotProp
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "fetch":
		err = runFetch(os.Args[2:])
	case "generate":
		err = runGenerate(os.Args[2:])
	case "refresh":
		err = runRefresh(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	default:
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "kis-specgen: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  kis-specgen fetch [flags]      # fetch snapshot from KIS portal")
	fmt.Fprintln(os.Stderr, "  kis-specgen generate [flags]   # generate Go files from snapshot")
	fmt.Fprintln(os.Stderr, "  kis-specgen refresh [flags]    # fetch + generate")
	fmt.Fprintln(os.Stderr, "  kis-specgen check [flags]      # verify generated files are up to date")
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	portalURL := fs.String("portal-url", defaultPortalURL, "KIS API guide URL")
	detailURL := fs.String("detail-url", defaultDetailURL, "KIS detail API URL")
	out := fs.String("out", defaultSnapshotPath, "snapshot JSON output path")
	workers := fs.Int("workers", 4, "parallel detail fetch workers")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workers <= 0 {
		return fmt.Errorf("workers must be > 0")
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *portalURL, *detailURL, *workers)
	if err != nil {
		return err
	}
	return writeSnapshot(*out, snap)
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "typed response Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snap, err := readSnapshot(*in)
	if err != nil {
		return err
	}

	specBytes, err := generateDocumentedSpecsGo(snap)
	if err != nil {
		return err
	}
	typesBytes, err := generateDocumentedTypesGo(snap)
	if err != nil {
		return err
	}

	if err := writeFile(*specOut, specBytes); err != nil {
		return err
	}
	if err := writeFile(*typesOut, typesBytes); err != nil {
		return err
	}
	return nil
}

func runRefresh(args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	portalURL := fs.String("portal-url", defaultPortalURL, "KIS API guide URL")
	detailURL := fs.String("detail-url", defaultDetailURL, "KIS detail API URL")
	snapshotPath := fs.String("snapshot", defaultSnapshotPath, "snapshot JSON path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "typed response Go output path")
	workers := fs.Int("workers", 4, "parallel detail fetch workers")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workers <= 0 {
		return fmt.Errorf("workers must be > 0")
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *portalURL, *detailURL, *workers)
	if err != nil {
		return err
	}
	if err := writeSnapshot(*snapshotPath, snap); err != nil {
		return err
	}

	specBytes, err := generateDocumentedSpecsGo(snap)
	if err != nil {
		return err
	}
	typesBytes, err := generateDocumentedTypesGo(snap)
	if err != nil {
		return err
	}
	if err := writeFile(*specOut, specBytes); err != nil {
		return err
	}
	if err := writeFile(*typesOut, typesBytes); err != nil {
		return err
	}
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "typed response Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snap, err := readSnapshot(*in)
	if err != nil {
		return err
	}

	wantSpecs, err := generateDocumentedSpecsGo(snap)
	if err != nil {
		return err
	}
	wantTypes, err := generateDocumentedTypesGo(snap)
	if err != nil {
		return err
	}

	if err := compareGenerated(*specOut, wantSpecs); err != nil {
		return fmt.Errorf("%w (run: go run ./cmd/kis-specgen generate)", err)
	}
	if err := compareGenerated(*typesOut, wantTypes); err != nil {
		return fmt.Errorf("%w (run: go run ./cmd/kis-specgen generate)", err)
	}
	return nil
}

func fetchSnapshot(client *http.Client, portalURL, detailURL string, workers int) (*snapshot, error) {
	paths, err := fetchDocumentedPaths(client, portalURL)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no /uapi paths discovered from %s", portalURL)
	}

	type result struct {
		endpoint endpointSnapshot
		err      error
	}

	ctx := context.Background()
	pathCh := make(chan string)
	resCh := make(chan result, len(paths))

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for p := range pathCh {
				ep, err := fetchEndpointDetail(ctx, client, detailURL, p)
				resCh <- result{endpoint: ep, err: err}
			}
		})
	}

	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)
	wg.Wait()
	close(resCh)

	endpoints := make([]endpointSnapshot, 0, len(paths))
	failures := make([]string, 0)
	for r := range resCh {
		if r.err != nil {
			failures = append(failures, r.err.Error())
			continue
		}
		endpoints = append(endpoints, r.endpoint)
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		return nil, fmt.Errorf("detail fetch failed (%d/%d): %s", len(failures), len(paths), strings.Join(failures, " | "))
	}

	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Path < endpoints[j].Path })
	return &snapshot{Endpoints: endpoints}, nil
}

func fetchDocumentedPaths(client *http.Client, portalURL string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, portalURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "krsec-kis-specgen/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetch portal page HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`goLeftMenuUrl\(&#39;(\/uapi\/[^&#]+)&#39;\)`)
	matches := re.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("could not parse /uapi paths from portal page")
	}

	uniq := map[string]struct{}{}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		path := strings.TrimSpace(htmlUnescape(m[1]))
		if path == "" {
			continue
		}
		uniq[path] = struct{}{}
	}

	paths := make([]string, 0, len(uniq))
	for p := range uniq {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

func htmlUnescape(in string) string {
	in = strings.ReplaceAll(in, "&amp;", "&")
	in = strings.ReplaceAll(in, "&quot;", "\"")
	in = strings.ReplaceAll(in, "&#39;", "'")
	return in
}

func fetchEndpointDetail(ctx context.Context, client *http.Client, detailURL, path string) (endpointSnapshot, error) {
	u, err := url.Parse(detailURL)
	if err != nil {
		return endpointSnapshot{}, fmt.Errorf("%s: invalid detail URL: %w", path, err)
	}
	q := u.Query()
	q.Set("accessUrl", path)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return endpointSnapshot{}, fmt.Errorf("%s: create request: %w", path, err)
	}
	req.Header.Set("User-Agent", "krsec-kis-specgen/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return endpointSnapshot{}, fmt.Errorf("%s: detail request: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return endpointSnapshot{}, fmt.Errorf("%s: detail HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw detailResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return endpointSnapshot{}, fmt.Errorf("%s: decode detail: %w", path, err)
	}

	if strings.TrimSpace(raw.AccessURL) == "" {
		return endpointSnapshot{}, fmt.Errorf("%s: empty accessUrl in detail response", path)
	}

	required := make([]string, 0, 32)
	requiredSet := map[string]struct{}{}
	resProps := make([]snapshotProp, 0, 256)
	resSeen := map[string]struct{}{}

	for _, p := range raw.APIPropertys {
		bodyType := strings.ToLower(strings.TrimSpace(p.BodyType))
		code := strings.TrimSpace(p.PropertyCd)
		typ := strings.TrimSpace(p.PropertyType)
		order := strings.TrimSpace(p.PropertyOrder)
		if code == "" {
			continue
		}

		switch bodyType {
		case "req_b":
			if strings.EqualFold(strings.TrimSpace(p.RequireYn), "Y") {
				if _, ok := requiredSet[code]; !ok {
					requiredSet[code] = struct{}{}
					required = append(required, code)
				}
			}
		case "res_b":
			k := order + "|" + code
			if _, ok := resSeen[k]; ok {
				continue
			}
			resSeen[k] = struct{}{}
			resProps = append(resProps, snapshotProp{
				Code:  code,
				Type:  typ,
				Order: order,
			})
		}
	}

	sort.Strings(required)
	sort.Slice(resProps, func(i, j int) bool {
		cmp := compareOrder(resProps[i].Order, resProps[j].Order)
		if cmp != 0 {
			return cmp < 0
		}
		return resProps[i].Code < resProps[j].Code
	})

	return endpointSnapshot{
		Path:           strings.TrimSpace(raw.AccessURL),
		Method:         strings.ToUpper(strings.TrimSpace(raw.HTTPMethod)),
		RealTRID:       strings.TrimSpace(raw.RealTRID),
		VirtualTRID:    normalizeVirtualTRID(raw.VirtualTRID),
		RequiredFields: required,
		ResponseProps:  resProps,
	}, nil
}

func normalizeVirtualTRID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "모의투자 미지원") {
		return ""
	}
	return v
}

func readSnapshot(path string) (*snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot %s: %w", path, err)
	}
	if len(snap.Endpoints) == 0 {
		return nil, fmt.Errorf("snapshot %s has no endpoints", path)
	}

	for i := range snap.Endpoints {
		ep := &snap.Endpoints[i]
		ep.Path = strings.TrimSpace(ep.Path)
		ep.Method = strings.ToUpper(strings.TrimSpace(ep.Method))
		ep.RealTRID = strings.TrimSpace(ep.RealTRID)
		ep.VirtualTRID = strings.TrimSpace(ep.VirtualTRID)
		for j := range ep.RequiredFields {
			ep.RequiredFields[j] = strings.TrimSpace(ep.RequiredFields[j])
		}
		sort.Strings(ep.RequiredFields)
		sort.Slice(ep.ResponseProps, func(a, b int) bool {
			cmp := compareOrder(ep.ResponseProps[a].Order, ep.ResponseProps[b].Order)
			if cmp != 0 {
				return cmp < 0
			}
			return ep.ResponseProps[a].Code < ep.ResponseProps[b].Code
		})
	}
	sort.Slice(snap.Endpoints, func(i, j int) bool { return snap.Endpoints[i].Path < snap.Endpoints[j].Path })
	return &snap, nil
}

func writeSnapshot(path string, snap *snapshot) error {
	if snap == nil {
		return errors.New("snapshot is nil")
	}
	sort.Slice(snap.Endpoints, func(i, j int) bool { return snap.Endpoints[i].Path < snap.Endpoints[j].Path })
	for i := range snap.Endpoints {
		sort.Strings(snap.Endpoints[i].RequiredFields)
		sort.Slice(snap.Endpoints[i].ResponseProps, func(a, b int) bool {
			cmp := compareOrder(snap.Endpoints[i].ResponseProps[a].Order, snap.Endpoints[i].ResponseProps[b].Order)
			if cmp != 0 {
				return cmp < 0
			}
			return snap.Endpoints[i].ResponseProps[a].Code < snap.Endpoints[i].ResponseProps[b].Code
		})
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func compareGenerated(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("generated file is stale: %s", path)
	}
	return nil
}

func generateDocumentedSpecsGo(snap *snapshot) ([]byte, error) {
	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/kis-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/kis/specs/documented_endpoints.json\n\n")
	b.WriteString("// KISEndpointSpec defines one KIS endpoint specification from official docs.\n")
	b.WriteString("type KISEndpointSpec struct {\n")
	b.WriteString("\tMethod         string\n")
	b.WriteString("\tRealTRID       string\n")
	b.WriteString("\tVirtualTRID    string\n")
	b.WriteString("\tRequiredFields []string\n")
	b.WriteString("}\n\n")
	b.WriteString("// DocumentedKISEndpointSpecs is generated from documented KIS snapshot.\n")
	b.WriteString("var DocumentedKISEndpointSpecs = map[string]KISEndpointSpec{\n")
	for _, ep := range snap.Endpoints {
		fmt.Fprintf(&b, "\t%q: {\n", ep.Path)
		fmt.Fprintf(&b, "\t\tMethod:         %q,\n", ep.Method)
		fmt.Fprintf(&b, "\t\tRealTRID:       %q,\n", ep.RealTRID)
		fmt.Fprintf(&b, "\t\tVirtualTRID:    %q,\n", ep.VirtualTRID)
		b.WriteString("\t\tRequiredFields: []string{")
		for i, f := range ep.RequiredFields {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", f)
		}
		b.WriteString("},\n")
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	return format.Source([]byte(b.String()))
}

func generateDocumentedTypesGo(snap *snapshot) ([]byte, error) {
	models := make([]endpointModel, 0, len(snap.Endpoints))
	for _, ep := range snap.Endpoints {
		respType := "KIS" + sanitizePath(ep.Path)
		models = append(models, endpointModel{
			Path:           ep.Path,
			TypeName:       respType,
			RequestType:    respType + "Request",
			RequiredFields: append([]string(nil), ep.RequiredFields...),
			Top:            buildResponseNodes(ep.ResponseProps),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Path < models[j].Path })

	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/kis-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/kis/specs/documented_endpoints.json\n\n")

	for _, m := range models {
		emitEndpointType(&b, m)
		emitEndpointRequestType(&b, m)
	}

	b.WriteString("var documentedEndpointResponseFactories = map[string]func() DocumentedEndpointResponse{\n")
	for _, m := range models {
		fmt.Fprintf(&b, "\t%q: func() DocumentedEndpointResponse { return &%s{} },\n", m.Path, m.TypeName)
	}
	b.WriteString("}\n")
	b.WriteString("\n")
	b.WriteString("var documentedEndpointRequestFactories = map[string]func() any{\n")
	for _, m := range models {
		fmt.Fprintf(&b, "\t%q: func() any { return &%s{} },\n", m.Path, m.RequestType)
	}
	b.WriteString("}\n")

	return format.Source([]byte(b.String()))
}

func emitEndpointRequestType(b *strings.Builder, m endpointModel) {
	fmt.Fprintf(b, "type %s struct {\n", m.RequestType)
	used := map[string]int{}
	usedJSONTags := map[string]struct{}{}
	for _, f := range m.RequiredFields {
		fieldCode := strings.TrimSpace(f)
		if fieldCode == "" {
			continue
		}
		if _, exists := usedJSONTags[fieldCode]; exists {
			continue
		}
		usedJSONTags[fieldCode] = struct{}{}
		fieldName := uniqueFieldName(toExportedIdentifier(fieldCode), used)
		fmt.Fprintf(b, "\t%s string `json:\"%s\"`\n", fieldName, fieldCode)
	}
	b.WriteString("}\n\n")
}

func buildResponseNodes(props []snapshotProp) []responseNode {
	topMap := map[string]snapshotProp{}
	childMap := map[string][]snapshotProp{}
	for _, p := range props {
		order := strings.TrimSpace(p.Order)
		if order == "" {
			continue
		}
		parts := strings.Split(order, ".")
		if len(parts) <= 1 {
			topMap[order] = p
			continue
		}
		parent := parts[0]
		if _, ok := topMap[parent]; !ok {
			topMap[parent] = snapshotProp{
				Code:  parentSyntheticCode(childMap[parent], p),
				Type:  "A0005",
				Order: parent,
			}
		}
		childMap[parent] = append(childMap[parent], p)
	}

	orders := make([]string, 0, len(topMap))
	for ord := range topMap {
		orders = append(orders, ord)
	}
	sort.Slice(orders, func(i, j int) bool {
		return compareOrder(orders[i], orders[j]) < 0
	})

	out := make([]responseNode, 0, len(orders))
	for _, ord := range orders {
		n := responseNode{
			Prop:     topMap[ord],
			Children: append([]snapshotProp(nil), childMap[ord]...),
		}
		sort.Slice(n.Children, func(i, j int) bool {
			cmp := compareOrder(n.Children[i].Order, n.Children[j].Order)
			if cmp != 0 {
				return cmp < 0
			}
			return n.Children[i].Code < n.Children[j].Code
		})
		out = append(out, n)
	}
	return out
}

func emitEndpointType(b *strings.Builder, m endpointModel) {
	fmt.Fprintf(b, "type %s struct {\n", m.TypeName)
	b.WriteString("\tDocumentedResponseBase\n")
	used := map[string]int{}
	usedTopJSONTags := map[string]struct{}{}
	childDefs := make([]struct {
		Name   string
		Fields []snapshotProp
	}, 0)

	for _, n := range m.Top {
		code := strings.TrimSpace(n.Prop.Code)
		if code == "" {
			continue
		}
		if code == "rt_cd" || code == "msg_cd" || code == "msg1" {
			continue
		}
		if _, exists := usedTopJSONTags[code]; exists {
			continue
		}
		usedTopJSONTags[code] = struct{}{}
		fieldName := uniqueFieldName(toExportedIdentifier(code), used)
		fieldType := goTypeForProp(n.Prop)
		if len(n.Children) > 0 {
			childName := m.TypeName + fieldName + "Item"
			fieldType = childName
			if isSliceContainer(n.Prop, code) {
				fieldType = "DocumentedSlice[" + childName + "]"
			}
			childDefs = append(childDefs, struct {
				Name   string
				Fields []snapshotProp
			}{Name: childName, Fields: n.Children})
		}
		fmt.Fprintf(b, "\t%s %s `json:\"%s,omitempty\"`\n", fieldName, fieldType, code)
	}
	b.WriteString("}\n\n")

	for _, c := range childDefs {
		fmt.Fprintf(b, "type %s struct {\n", c.Name)
		usedChild := map[string]int{}
		usedChildJSONTags := map[string]struct{}{}
		for _, p := range c.Fields {
			code := strings.TrimSpace(p.Code)
			if code == "" {
				continue
			}
			if _, exists := usedChildJSONTags[code]; exists {
				continue
			}
			usedChildJSONTags[code] = struct{}{}
			fieldName := uniqueFieldName(toExportedIdentifier(code), usedChild)
			fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", fieldName, goTypeForProp(p), code)
		}
		b.WriteString("}\n\n")
	}
}

func isSliceContainer(p snapshotProp, code string) bool {
	pt := strings.ToUpper(strings.TrimSpace(p.Type))
	if pt == "A0005" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "output", "output1", "output2", "output3", "output4":
		return true
	default:
		return false
	}
}

func goTypeForProp(p snapshotProp) string {
	switch strings.ToUpper(strings.TrimSpace(p.Type)) {
	case "A0005":
		return "[]map[string]any"
	default:
		// KIS response values are often encoded as strings regardless of display type.
		return "string"
	}
}

func sanitizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return "Unknown"
	}
	if strings.EqualFold(parts[0], "uapi") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "Unknown"
	}

	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(toExportedIdentifier(p))
	}
	out := b.String()
	if out == "" {
		return "Unknown"
	}
	if unicode.IsDigit(rune(out[0])) {
		return "N" + out
	}
	return out
}

func toExportedIdentifier(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "Field"
	}
	toks := strings.FieldsFunc(in, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(toks) == 0 {
		return "Field"
	}

	var b strings.Builder
	for _, t := range toks {
		if t == "" {
			continue
		}
		lt := strings.ToLower(t)
		b.WriteString(strings.ToUpper(lt[:1]))
		if len(lt) > 1 {
			b.WriteString(lt[1:])
		}
	}

	out := b.String()
	if out == "" {
		out = "Field"
	}
	if unicode.IsDigit(rune(out[0])) {
		out = "N" + out
	}
	switch out {
	case "Type", "Map", "Var", "Func", "Interface", "Struct", "Range", "Select", "Case", "Default", "Package":
		out += "Field"
	}
	return out
}

func uniqueFieldName(base string, used map[string]int) string {
	if base == "" {
		base = "Field"
	}
	if _, ok := used[base]; !ok {
		used[base] = 1
		return base
	}
	used[base]++
	return base + strconv.Itoa(used[base])
}

func compareOrder(a, b string) int {
	pa := parseOrder(a)
	pb := parseOrder(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	if len(pa) < len(pb) {
		return -1
	}
	if len(pa) > len(pb) {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func parseOrder(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return []int{0}
	}
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

func parentSyntheticCode(existing []snapshotProp, child snapshotProp) string {
	for _, p := range existing {
		if strings.TrimSpace(p.Code) != "" {
			return strings.TrimSpace(p.Code)
		}
	}
	if strings.Contains(strings.ToLower(child.Code), "output") {
		return "output"
	}
	return "container"
}
