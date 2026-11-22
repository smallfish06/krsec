package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultGuideURL is the official LS API guide file offered for download
	// on every openapi.ls-sec.co.kr page.
	defaultGuideURL       = "https://openapi.ls-sec.co.kr/api/apis/public/tr-guides.json"
	defaultSnapshotPath   = "pkg/ls/specs/documented_endpoints.json"
	defaultSpecsOutPath   = "pkg/ls/specs/documented_specs_generated.go"
	documentFetchAttempts = 3
	maxDocumentBytes      = 16 << 20
)

// lsFieldTypeCodes maps the guide's field type names to the portal type codes
// that LSFieldSpec.Type exposes.
var lsFieldTypeCodes = map[string]string{
	"String":       "A0001",
	"Object":       "A0003",
	"Number":       "A0004",
	"Object Array": "A0005",
}

type snapshot struct {
	Source    string             `json:"source,omitempty"`
	Endpoints []endpointSnapshot `json:"endpoints"`
}

type endpointSnapshot struct {
	GroupName       string       `json:"group_name,omitempty"`
	APIName         string       `json:"api_name"`
	Path            string       `json:"path"`
	Method          string       `json:"method"`
	Protocol        string       `json:"protocol"`
	Domain          string       `json:"domain,omitempty"`
	SimulatedDomain string       `json:"simulated_domain,omitempty"`
	Description     string       `json:"description,omitempty"`
	TRs             []trSnapshot `json:"trs"`
}

type trSnapshot struct {
	Name              string             `json:"name"`
	Code              string             `json:"code"`
	TransactionPerSec string             `json:"transaction_per_sec,omitempty"`
	Properties        []propertySnapshot `json:"properties,omitempty"`
}

type propertySnapshot struct {
	BodyType    string `json:"body_type"`
	Code        string `json:"code"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Length      string `json:"length,omitempty"`
	Order       string `json:"order,omitempty"`
	Description string `json:"description,omitempty"`
}

// guideDocument is the subset of tr-guides.json the generator reads.
type guideDocument struct {
	Defaults guideDefaults `json:"defaults"`
	Servers  []guideServer `json:"servers"`
	APIs     []guideAPI    `json:"apis"`
}

type guideDefaults struct {
	Method        string `json:"method"`
	FieldRequired *bool  `json:"fieldRequired"`
}

type guideServer struct {
	Protocol      string `json:"protocol"`
	URL           string `json:"url"`
	SimulationURL string `json:"simulationUrl"`
}

type guideAPI struct {
	Categories    []string           `json:"categories"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Protocol      string             `json:"protocol"`
	Path          string             `json:"path"`
	Method        string             `json:"method"`
	CommonHeaders guideHeaders       `json:"commonHeaders"`
	Transactions  []guideTransaction `json:"transactions"`
}

type guideHeaders struct {
	Request  []guideField `json:"request"`
	Response []guideField `json:"response"`
}

type guideTransaction struct {
	Code              string       `json:"code"`
	Name              string       `json:"name"`
	TransactionPerSec looseString  `json:"transactionPerSec"`
	Request           guideMessage `json:"request"`
	Response          guideMessage `json:"response"`
}

type guideMessage struct {
	Headers []guideField `json:"headers"`
	Body    []guideField `json:"body"`
}

type guideField struct {
	Element  string       `json:"element"`
	Name     string       `json:"name"`
	Type     string       `json:"type"`
	Length   looseString  `json:"length"`
	Required *bool        `json:"required"`
	Fields   []guideField `json:"fields"`
}

type looseString string

func (s *looseString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = looseString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*s = looseString(number.String())
		return nil
	}
	return nil
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
		fmt.Fprintf(os.Stderr, "ls-specgen: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  ls-specgen fetch [flags]      # fetch snapshot from LS OpenAPI guide")
	fmt.Fprintln(os.Stderr, "  ls-specgen generate [flags]   # generate Go files from snapshot")
	fmt.Fprintln(os.Stderr, "  ls-specgen refresh [flags]    # fetch + generate")
	fmt.Fprintln(os.Stderr, "  ls-specgen check [flags]      # verify generated files are up to date")
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	guideURL := fs.String("guide-url", defaultGuideURL, "LS API guide JSON URL")
	out := fs.String("out", defaultSnapshotPath, "snapshot JSON output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *guideURL)
	if err != nil {
		return err
	}
	return writeSnapshot(*out, snap)
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
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
	return writeFile(*specOut, specBytes)
}

func runRefresh(args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	guideURL := fs.String("guide-url", defaultGuideURL, "LS API guide JSON URL")
	snapshotPath := fs.String("snapshot", defaultSnapshotPath, "snapshot JSON path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *guideURL)
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
	return writeFile(*specOut, specBytes)
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
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
	if err := compareGenerated(*specOut, wantSpecs); err != nil {
		return fmt.Errorf("%w (run: go run ./cmd/ls-specgen generate)", err)
	}
	return nil
}

func fetchSnapshot(client *http.Client, guideURL string) (*snapshot, error) {
	data, err := getBytes(client, guideURL)
	if err != nil {
		return nil, err
	}
	var doc guideDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode LS API guide %s: %w", guideURL, err)
	}
	snap, err := snapshotFromGuide(&doc)
	if err != nil {
		return nil, fmt.Errorf("LS API guide %s: %w", guideURL, err)
	}
	snap.Source = guideURL
	return snap, nil
}

// snapshotFromGuide converts the guide document into the committed snapshot
// layout: one endpoint per guide API, one TR per transaction.
func snapshotFromGuide(doc *guideDocument) (*snapshot, error) {
	fieldRequired := true
	if doc.Defaults.FieldRequired != nil {
		fieldRequired = *doc.Defaults.FieldRequired
	}
	servers := make(map[string]guideServer, len(doc.Servers))
	for _, server := range doc.Servers {
		servers[strings.ToUpper(strings.TrimSpace(server.Protocol))] = server
	}

	var endpoints []endpointSnapshot
	for _, api := range doc.APIs {
		path := normalizePath(api.Path)
		protocol := strings.ToUpper(strings.TrimSpace(api.Protocol))
		if path == "" || protocol == "" {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(api.Method))
		if method == "" {
			method = strings.ToUpper(strings.TrimSpace(doc.Defaults.Method))
		}
		server := servers[protocol]
		ep := endpointSnapshot{
			GroupName:       firstNonEmpty(api.Categories),
			APIName:         strings.TrimSpace(api.Name),
			Path:            path,
			Method:          method,
			Protocol:        protocol,
			Domain:          strings.TrimRight(strings.TrimSpace(server.URL), "/"),
			SimulatedDomain: strings.TrimRight(strings.TrimSpace(server.SimulationURL), "/"),
			Description:     strings.TrimSpace(api.Description),
		}
		for _, tx := range api.Transactions {
			code := strings.TrimSpace(tx.Code)
			if code == "" {
				continue
			}
			props, err := transactionProperties(api.CommonHeaders, tx, fieldRequired)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", path, code, err)
			}
			ep.TRs = append(ep.TRs, trSnapshot{
				Name:              strings.TrimSpace(tx.Name),
				Code:              code,
				TransactionPerSec: strings.TrimSpace(string(tx.TransactionPerSec)),
				Properties:        props,
			})
		}
		if len(ep.TRs) > 0 {
			endpoints = append(endpoints, ep)
		}
	}
	if len(endpoints) == 0 {
		return nil, errors.New("no LS TRs found")
	}
	sortEndpoints(endpoints)
	return &snapshot{Endpoints: endpoints}, nil
}

// transactionProperties flattens one transaction's headers and body into
// snapshot properties. A transaction without headers of its own uses the
// API's common headers.
func transactionProperties(common guideHeaders, tx guideTransaction, fieldRequired bool) ([]propertySnapshot, error) {
	requestHeaders := tx.Request.Headers
	if len(requestHeaders) == 0 {
		requestHeaders = common.Request
	}
	responseHeaders := tx.Response.Headers
	if len(responseHeaders) == 0 {
		responseHeaders = common.Response
	}

	var props []propertySnapshot
	for _, part := range []struct {
		bodyType string
		fields   []guideField
	}{
		{"req_h", requestHeaders},
		{"req_b", tx.Request.Body},
		{"res_h", responseHeaders},
		{"res_b", tx.Response.Body},
	} {
		var err error
		props, err = appendFieldProperties(props, part.bodyType, "", part.fields, fieldRequired)
		if err != nil {
			return nil, err
		}
	}
	return props, nil
}

// appendFieldProperties appends fields in document order. Order is the
// 1-based position, dotted under the parent block ("001", "001.002").
func appendFieldProperties(props []propertySnapshot, bodyType, parentOrder string, fields []guideField, fieldRequired bool) ([]propertySnapshot, error) {
	for i, field := range fields {
		code := cleanPropertyCode(field.Element)
		if code == "" {
			continue
		}
		order := fmt.Sprintf("%03d", i+1)
		if parentOrder != "" {
			order = parentOrder + "." + order
		}
		var typeCode string
		if name := strings.TrimSpace(field.Type); name != "" {
			var ok bool
			if typeCode, ok = lsFieldTypeCodes[name]; !ok {
				return nil, fmt.Errorf("field %s: unknown LS field type %q", code, name)
			}
		}
		required := fieldRequired
		if field.Required != nil {
			required = *field.Required
		}
		props = append(props, propertySnapshot{
			BodyType: bodyType,
			Code:     code,
			Name:     strings.TrimSpace(field.Name),
			Type:     typeCode,
			Required: required,
			Length:   strings.TrimSpace(string(field.Length)),
			Order:    order,
		})
		var err error
		props, err = appendFieldProperties(props, bodyType, order, field.Fields, fieldRequired)
		if err != nil {
			return nil, err
		}
	}
	return props, nil
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func getBytes(client *http.Client, url string) ([]byte, error) {
	return getBytesWithRetry(client, url, time.Second)
}

func getBytesWithRetry(client *http.Client, url string, retryDelay time.Duration) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		data, retry, err := getBytesOnce(client, url)
		if err == nil {
			return data, nil
		}
		if !retry || attempt == documentFetchAttempts {
			return nil, fmt.Errorf("GET %s failed after %d attempt(s): %w", url, attempt, err)
		}
		time.Sleep(retryDelay * time.Duration(attempt))
	}
}

func getBytesOnce(client *http.Client, url string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "krsec-ls-specgen/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	responseInfo := fmt.Sprintf("HTTP %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		// Error pages can contain session identifiers; never include their body.
		return nil, retry, fmt.Errorf("fetch document: %s", responseInfo)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("read document (%s): %w", responseInfo, err)
	}
	if len(data) > maxDocumentBytes {
		return nil, false, fmt.Errorf("document exceeds %d bytes (%s)", maxDocumentBytes, responseInfo)
	}
	return data, false, nil
}

func sortEndpoints(endpoints []endpointSnapshot) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		a, b := endpoints[i], endpoints[j]
		if a.GroupName != b.GroupName {
			return a.GroupName < b.GroupName
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.APIName < b.APIName
	})
	for epIdx := range endpoints {
		trs := endpoints[epIdx].TRs
		// Codes that differ only in case (KOSPI "S3_" vs ELW "s3_") share one
		// generated key, so break ties on the exact code to keep the winner stable.
		sort.SliceStable(trs, func(i, j int) bool {
			a, b := strings.ToLower(trs[i].Code), strings.ToLower(trs[j].Code)
			if a != b {
				return a < b
			}
			return trs[i].Code < trs[j].Code
		})
	}
}

func readSnapshot(path string) (*snapshot, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from a CLI flag
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return &snap, nil
}

func writeSnapshot(path string, snap *snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

// Generated artifacts are repo files; standard 0o755/0o644 permissions are intended.
//
//nolint:gosec // G301/G306: output paths come from CLI flags and land in the repo
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func compareGenerated(path string, want []byte) error {
	got, err := os.ReadFile(path) //nolint:gosec // G304: path comes from a CLI flag
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return errors.New("generated file is out of date")
	}
	return nil
}

func generateDocumentedSpecsGo(snap *snapshot) ([]byte, error) {
	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/ls-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/ls/specs/documented_endpoints.json\n\n")
	b.WriteString("// DocumentedLSEndpointSpecs is generated from documented LS snapshot.\n")
	b.WriteString("var DocumentedLSEndpointSpecs = map[string]LSEndpointSpec{\n")

	type generatedSpec struct {
		key  string
		path string
		tr   trSnapshot
		ep   endpointSnapshot
	}
	var specs []generatedSpec
	seen := make(map[string]struct{})
	for _, ep := range snap.Endpoints {
		path := normalizePath(ep.Path)
		for _, tr := range ep.TRs {
			code := strings.TrimSpace(tr.Code)
			if path == "" || code == "" {
				continue
			}
			key := documentedEndpointKey(path, code)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			specs = append(specs, generatedSpec{key: key, path: path, tr: tr, ep: ep})
		}
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].key < specs[j].key
	})
	for _, item := range specs {
		b.WriteString("\t")
		b.WriteString(quote(item.key))
		b.WriteString(": {\n")
		b.WriteString("\t\tPath: ")
		b.WriteString(quote(item.path))
		b.WriteString(",\n\t\tTRCode: ")
		b.WriteString(quote(item.tr.Code))
		b.WriteString(",\n\t\tMethod: ")
		b.WriteString(quote(defaultMethod(item.ep.Method)))
		b.WriteString(",\n\t\tProtocol: ")
		b.WriteString(quote(strings.ToUpper(strings.TrimSpace(item.ep.Protocol))))
		b.WriteString(",\n\t\tGroupName: ")
		b.WriteString(quote(item.ep.GroupName))
		b.WriteString(",\n\t\tAPIName: ")
		b.WriteString(quote(item.ep.APIName))
		b.WriteString(",\n\t\tTRName: ")
		b.WriteString(quote(item.tr.Name))
		b.WriteString(",\n\t\tTransactionPerSec: ")
		b.WriteString(quote(item.tr.TransactionPerSec))
		b.WriteString(",\n")
		writeFieldSpecs(&b, "RequestHeaders", filterProperties(item.tr.Properties, "req_h"))
		writeFieldSpecs(&b, "RequestFields", filterProperties(item.tr.Properties, "req_b"))
		writeFieldSpecs(&b, "ResponseHeaders", filterProperties(item.tr.Properties, "res_h"))
		writeFieldSpecs(&b, "ResponseFields", filterProperties(item.tr.Properties, "res_b"))
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")

	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, fmt.Errorf("format generated specs: %w", err)
	}
	return out, nil
}

func writeFieldSpecs(b *strings.Builder, name string, fields []propertySnapshot) {
	b.WriteString("\t\t")
	b.WriteString(name)
	b.WriteString(": []LSFieldSpec{\n")
	for _, field := range fields {
		b.WriteString("\t\t\t{Code: ")
		b.WriteString(quote(field.Code))
		if field.Name != "" {
			b.WriteString(", Name: ")
			b.WriteString(quote(field.Name))
		}
		if field.Type != "" {
			b.WriteString(", Type: ")
			b.WriteString(quote(field.Type))
		}
		if field.Length != "" {
			b.WriteString(", Length: ")
			b.WriteString(quote(field.Length))
		}
		if field.Order != "" {
			b.WriteString(", Order: ")
			b.WriteString(quote(field.Order))
		}
		if field.Required {
			b.WriteString(", Required: true")
		}
		if field.Description != "" {
			b.WriteString(", Description: ")
			b.WriteString(quote(field.Description))
		}
		b.WriteString("},\n")
	}
	b.WriteString("\t\t},\n")
}

func filterProperties(props []propertySnapshot, bodyType string) []propertySnapshot {
	out := make([]propertySnapshot, 0)
	for _, prop := range props {
		if strings.EqualFold(strings.TrimSpace(prop.BodyType), bodyType) {
			out = append(out, prop)
		}
	}
	return out
}

func defaultMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return http.MethodPost
	}
	return method
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

func cleanPropertyCode(code string) string {
	code = html.UnescapeString(code)
	code = strings.ReplaceAll(code, "\u00a0", " ")
	code = strings.TrimSpace(code)
	code = strings.TrimLeft(code, "- ")
	code = strings.TrimSpace(code)
	return code
}

func quote(s string) string {
	return strconv.QuoteToASCII(s)
}
