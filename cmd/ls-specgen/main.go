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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPortalURL    = "https://openapi.ls-sec.co.kr/apiservice"
	defaultAPIListURL   = "https://openapi.ls-sec.co.kr/api/apis/public/api-list"
	defaultTRListURL    = "https://openapi.ls-sec.co.kr/api/apis/guide/tr"
	defaultPropertyURL  = "https://openapi.ls-sec.co.kr/api/apis/guide/tr/property"
	defaultSnapshotPath = "pkg/ls/specs/documented_endpoints.json"
	defaultSpecsOutPath = "pkg/ls/specs/documented_specs_generated.go"
)

type snapshot struct {
	Source    string             `json:"source,omitempty"`
	FetchedAt string             `json:"fetched_at,omitempty"`
	Groups    []groupSnapshot    `json:"groups,omitempty"`
	Endpoints []endpointSnapshot `json:"endpoints"`
}

type groupSnapshot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type endpointSnapshot struct {
	GroupID         string       `json:"group_id,omitempty"`
	GroupName       string       `json:"group_name,omitempty"`
	APIID           string       `json:"api_id"`
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
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Code              string             `json:"code"`
	TransactionPerSec string             `json:"transaction_per_sec,omitempty"`
	ReqExample        string             `json:"req_example,omitempty"`
	ResExample        string             `json:"res_example,omitempty"`
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

type apiRecord struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	AccessURL       string      `json:"accessUrl"`
	Description     looseString `json:"description"`
	HTTPMethod      string      `json:"httpMethod"`
	Domain          string      `json:"domain"`
	ProtocolType    string      `json:"protocolType"`
	SimulatedDomain looseString `json:"simulatedDomain"`
	Open            bool        `json:"open"`
}

type trRecord struct {
	ID                string      `json:"id"`
	TRName            string      `json:"trName"`
	TRCode            string      `json:"trCode"`
	TransactionPerSec looseString `json:"transactionPerSec"`
	ReqExample        looseString `json:"reqExample"`
	ResExample        looseString `json:"resExample"`
}

type propertyRecord struct {
	BodyType       string      `json:"bodyType"`
	PropertyCD     looseString `json:"propertyCd"`
	PropertyName   looseString `json:"propertyNm"`
	PropertyType   looseString `json:"propertyType"`
	PropertyLength looseString `json:"propertyLength"`
	PropertyOrder  looseString `json:"propertyOrder"`
	RequireYN      looseString `json:"requireYn"`
	Description    looseString `json:"description"`
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
	portalURL := fs.String("portal-url", defaultPortalURL, "LS API guide URL")
	apiListURL := fs.String("api-list-url", defaultAPIListURL, "LS API list base URL")
	trListURL := fs.String("tr-list-url", defaultTRListURL, "LS TR list base URL")
	propertyURL := fs.String("property-url", defaultPropertyURL, "LS TR property base URL")
	out := fs.String("out", defaultSnapshotPath, "snapshot JSON output path")
	workers := fs.Int("workers", 8, "parallel property fetch workers")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workers <= 0 {
		return fmt.Errorf("workers must be > 0")
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *portalURL, *apiListURL, *trListURL, *propertyURL, *workers)
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
	portalURL := fs.String("portal-url", defaultPortalURL, "LS API guide URL")
	apiListURL := fs.String("api-list-url", defaultAPIListURL, "LS API list base URL")
	trListURL := fs.String("tr-list-url", defaultTRListURL, "LS TR list base URL")
	propertyURL := fs.String("property-url", defaultPropertyURL, "LS TR property base URL")
	snapshotPath := fs.String("snapshot", defaultSnapshotPath, "snapshot JSON path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	workers := fs.Int("workers", 8, "parallel property fetch workers")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workers <= 0 {
		return fmt.Errorf("workers must be > 0")
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *portalURL, *apiListURL, *trListURL, *propertyURL, *workers)
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

func fetchSnapshot(
	client *http.Client,
	portalURL string,
	apiListURL string,
	trListURL string,
	propertyURL string,
	workers int,
) (*snapshot, error) {
	groups, err := fetchGroups(client, portalURL)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no LS API groups discovered from %s", portalURL)
	}

	var endpoints []endpointSnapshot
	for _, group := range groups {
		records, err := fetchAPIList(client, apiListURL, group.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch API list for %s: %w", group.ID, err)
		}
		for _, record := range records {
			path := normalizePath(record.AccessURL)
			apiID := strings.TrimSpace(record.ID)
			if apiID == "" || path == "" {
				continue
			}
			trs, err := fetchTRList(client, trListURL, apiID)
			if err != nil {
				return nil, fmt.Errorf("fetch TR list for %s %s: %w", path, apiID, err)
			}
			if len(trs) == 0 {
				continue
			}
			endpoints = append(endpoints, endpointSnapshot{
				GroupID:         group.ID,
				GroupName:       group.Name,
				APIID:           apiID,
				APIName:         strings.TrimSpace(record.Name),
				Path:            path,
				Method:          strings.ToUpper(strings.TrimSpace(record.HTTPMethod)),
				Protocol:        strings.ToUpper(strings.TrimSpace(record.ProtocolType)),
				Domain:          strings.TrimRight(strings.TrimSpace(record.Domain), "/"),
				SimulatedDomain: strings.TrimRight(strings.TrimSpace(string(record.SimulatedDomain)), "/"),
				Description:     strings.TrimSpace(string(record.Description)),
				TRs:             trs,
			})
		}
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no LS TRs discovered from %s", portalURL)
	}

	if err := fetchProperties(client, propertyURL, endpoints, workers); err != nil {
		return nil, err
	}
	sortSnapshot(groups, endpoints)
	return &snapshot{
		Source:    portalURL,
		Groups:    groups,
		Endpoints: endpoints,
	}, nil
}

func fetchGroups(client *http.Client, portalURL string) ([]groupSnapshot, error) {
	body, err := getBytes(client, portalURL)
	if err != nil {
		return nil, err
	}
	text := html.UnescapeString(string(body))
	re := regexp.MustCompile(`loadApiList\("([^"]+)",\s*"((?:\\.|[^"])*)"\)`)
	matches := re.FindAllStringSubmatch(text, -1)
	seen := make(map[string]struct{}, len(matches))
	groups := make([]groupSnapshot, 0, len(matches))
	for _, m := range matches {
		id := strings.TrimSpace(m[1])
		name := decodeJSONString(m[2])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		groups = append(groups, groupSnapshot{ID: id, Name: strings.TrimSpace(name)})
	}
	return groups, nil
}

func fetchAPIList(client *http.Client, apiListURL, groupID string) ([]apiRecord, error) {
	var out []apiRecord
	if err := getJSON(client, joinURL(apiListURL, groupID), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchTRList(client *http.Client, trListURL, apiID string) ([]trSnapshot, error) {
	data, err := getBytes(client, joinURL(trListURL, apiID))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	var records []trRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode TR list: %w", err)
	}
	trs := make([]trSnapshot, 0, len(records))
	for _, record := range records {
		id := strings.TrimSpace(record.ID)
		code := strings.TrimSpace(record.TRCode)
		if id == "" || code == "" {
			continue
		}
		trs = append(trs, trSnapshot{
			ID:                id,
			Name:              strings.TrimSpace(record.TRName),
			Code:              code,
			TransactionPerSec: strings.TrimSpace(string(record.TransactionPerSec)),
		})
	}
	sort.Slice(trs, func(i, j int) bool {
		return strings.ToLower(trs[i].Code) < strings.ToLower(trs[j].Code)
	})
	return trs, nil
}

func fetchProperties(client *http.Client, propertyURL string, endpoints []endpointSnapshot, workers int) error {
	type job struct {
		endpoint int
		tr       int
		id       string
	}
	type result struct {
		endpoint   int
		tr         int
		properties []propertySnapshot
		err        error
	}

	jobs := make(chan job)
	results := make(chan result)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for j := range jobs {
				props, err := fetchTRProperties(client, propertyURL, j.id)
				results <- result{endpoint: j.endpoint, tr: j.tr, properties: props, err: err}
			}
		})
	}

	total := 0
	go func() {
		for epIdx := range endpoints {
			for trIdx := range endpoints[epIdx].TRs {
				total++
				jobs <- job{endpoint: epIdx, tr: trIdx, id: endpoints[epIdx].TRs[trIdx].ID}
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var firstErr error
	count := 0
	for res := range results {
		count++
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
		if res.err == nil {
			endpoints[res.endpoint].TRs[res.tr].Properties = res.properties
		}
	}
	if firstErr != nil {
		return firstErr
	}
	if count != total {
		return fmt.Errorf("property fetch count mismatch: got %d want %d", count, total)
	}
	return nil
}

func fetchTRProperties(client *http.Client, propertyURL, trID string) ([]propertySnapshot, error) {
	data, err := getBytes(client, joinURL(propertyURL, trID))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	var records []propertyRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode TR properties: %w", err)
	}
	props := make([]propertySnapshot, 0, len(records))
	for _, record := range records {
		code := cleanPropertyCode(string(record.PropertyCD))
		bodyType := strings.ToLower(strings.TrimSpace(record.BodyType))
		if code == "" || bodyType == "" {
			continue
		}
		props = append(props, propertySnapshot{
			BodyType: bodyType,
			Code:     code,
			Name:     strings.TrimSpace(string(record.PropertyName)),
			Type:     strings.TrimSpace(string(record.PropertyType)),
			Required: strings.EqualFold(strings.TrimSpace(string(record.RequireYN)), "Y"),
			Length:   cleanPropertyLength(string(record.PropertyLength)),
			Order:    strings.TrimSpace(string(record.PropertyOrder)),
		})
	}
	sort.SliceStable(props, func(i, j int) bool {
		return props[i].Order < props[j].Order
	})
	return props, nil
}

func getJSON(client *http.Client, url string, out any) error {
	data, err := getBytes(client, url)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func getBytes(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json,text/html;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "krsec-ls-specgen/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func sortSnapshot(groups []groupSnapshot, endpoints []endpointSnapshot) {
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
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
		return a.APIID < b.APIID
	})
	for epIdx := range endpoints {
		sort.SliceStable(endpoints[epIdx].TRs, func(i, j int) bool {
			return strings.ToLower(endpoints[epIdx].TRs[i].Code) < strings.ToLower(endpoints[epIdx].TRs[j].Code)
		})
	}
}

func readSnapshot(path string) (*snapshot, error) {
	data, err := os.ReadFile(path)
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

func joinURL(base, id string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/") + "/" + strings.TrimLeft(strings.TrimSpace(id), "/")
}

func cleanPropertyCode(code string) string {
	code = html.UnescapeString(code)
	code = strings.ReplaceAll(code, "\u00a0", " ")
	code = strings.TrimSpace(code)
	code = strings.TrimLeft(code, "- ")
	code = strings.TrimSpace(code)
	return code
}

// cleanPropertyLength normalizes the portal's propertyLength: the API emits
// the literal string "null" (not JSON null) for block-header rows.
func cleanPropertyLength(raw string) string {
	length := strings.TrimSpace(raw)
	if strings.EqualFold(length, "null") {
		return ""
	}
	return length
}

func decodeJSONString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	out, err := strconv.Unquote(`"` + raw + `"`)
	if err != nil {
		return raw
	}
	return out
}

func quote(s string) string {
	return strconv.QuoteToASCII(s)
}
