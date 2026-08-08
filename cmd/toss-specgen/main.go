package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
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
	defaultOpenAPIURL   = "https://openapi.tossinvest.com/openapi-docs/latest/openapi.json"
	defaultSnapshotPath = "pkg/toss/specs/documented_endpoints.json"
	defaultSpecsOutPath = "pkg/toss/specs/documented_specs_generated.go"
)

type openAPISnapshot struct {
	OpenAPI    string                          `json:"openapi"`
	Info       map[string]any                  `json:"info"`
	Servers    []map[string]any                `json:"servers,omitempty"`
	Paths      map[string]map[string]operation `json:"paths"`
	Components map[string]any                  `json:"components,omitempty"`
}

type operation struct {
	Tags        []string    `json:"tags,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Description string      `json:"description,omitempty"`
	OperationID string      `json:"operationId,omitempty"`
	Parameters  []parameter `json:"parameters,omitempty"`
}

type parameter struct {
	Ref  string `json:"$ref,omitempty"`
	Name string `json:"name,omitempty"`
	In   string `json:"in,omitempty"`
}

type endpointSpec struct {
	Path            string
	Method          string
	OperationID     string
	Tag             string
	Summary         string
	RateLimitGroup  string
	AccountRequired bool
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
		fmt.Fprintf(os.Stderr, "toss-specgen: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  toss-specgen fetch [flags]      # fetch Toss OpenAPI JSON snapshot")
	fmt.Fprintln(os.Stderr, "  toss-specgen generate [flags]   # generate Go files from snapshot")
	fmt.Fprintln(os.Stderr, "  toss-specgen refresh [flags]    # fetch + generate")
	fmt.Fprintln(os.Stderr, "  toss-specgen check [flags]      # verify generated files are up to date")
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	url := fs.String("url", defaultOpenAPIURL, "Toss OpenAPI JSON URL")
	out := fs.String("out", defaultSnapshotPath, "snapshot JSON output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *url)
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
	url := fs.String("url", defaultOpenAPIURL, "Toss OpenAPI JSON URL")
	snapshotPath := fs.String("snapshot", defaultSnapshotPath, "snapshot JSON path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "documented specs Go output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *url)
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
		return fmt.Errorf("%w (run: go run ./cmd/toss-specgen generate)", err)
	}
	return nil
}

func fetchSnapshot(client *http.Client, url string) (*openAPISnapshot, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "krsec-toss-specgen/1.0")

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

	var snap openAPISnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode Toss OpenAPI JSON: %w", err)
	}
	if len(snap.Paths) == 0 {
		return nil, fmt.Errorf("toss OpenAPI snapshot contains no paths")
	}
	return &snap, nil
}

func readSnapshot(path string) (*openAPISnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap openAPISnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return &snap, nil
}

func writeSnapshot(path string, snap *openAPISnapshot) error {
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

func generateDocumentedSpecsGo(snap *openAPISnapshot) ([]byte, error) {
	specs := collectEndpointSpecs(snap)
	if len(specs) == 0 {
		return nil, fmt.Errorf("no Toss endpoint specs discovered")
	}

	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/toss-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/toss/specs/documented_endpoints.json\n\n")
	b.WriteString("// DocumentedTossEndpointSpecs is generated from the Toss OpenAPI snapshot.\n")
	b.WriteString("var DocumentedTossEndpointSpecs = map[string]TossEndpointSpec{\n")
	for _, spec := range specs {
		key := strings.ToUpper(spec.Method) + " " + normalizePath(spec.Path)
		b.WriteString("\t")
		b.WriteString(strconv.Quote(key))
		b.WriteString(": {\n")
		b.WriteString("\t\tPath: ")
		b.WriteString(strconv.Quote(normalizePath(spec.Path)))
		b.WriteString(",\n\t\tMethod: ")
		b.WriteString(strconv.Quote(strings.ToUpper(spec.Method)))
		b.WriteString(",\n\t\tOperationID: ")
		b.WriteString(strconv.Quote(spec.OperationID))
		b.WriteString(",\n\t\tTag: ")
		b.WriteString(strconv.Quote(spec.Tag))
		b.WriteString(",\n\t\tSummary: ")
		b.WriteString(strconv.Quote(spec.Summary))
		b.WriteString(",\n\t\tRateLimitGroup: ")
		b.WriteString(strconv.Quote(spec.RateLimitGroup))
		b.WriteString(",\n\t\tAccountRequired: ")
		if spec.AccountRequired {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(",\n\t},\n")
	}
	b.WriteString("}\n")

	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func collectEndpointSpecs(snap *openAPISnapshot) []endpointSpec {
	methods := map[string]struct{}{
		"get": {}, "post": {}, "put": {}, "delete": {}, "patch": {},
	}
	specs := make([]endpointSpec, 0)
	for path, operations := range snap.Paths {
		for method, op := range operations {
			if _, ok := methods[strings.ToLower(method)]; !ok {
				continue
			}
			tag := ""
			if len(op.Tags) > 0 {
				tag = strings.TrimSpace(op.Tags[0])
			}
			specs = append(specs, endpointSpec{
				Path:            normalizePath(path),
				Method:          strings.ToUpper(method),
				OperationID:     strings.TrimSpace(op.OperationID),
				Tag:             tag,
				Summary:         strings.TrimSpace(op.Summary),
				RateLimitGroup:  extractRateLimitGroup(op.Description),
				AccountRequired: operationRequiresAccount(op),
			})
		}
	}
	sort.SliceStable(specs, func(i, j int) bool {
		if specs[i].Path != specs[j].Path {
			return specs[i].Path < specs[j].Path
		}
		return specs[i].Method < specs[j].Method
	})
	return specs
}

func extractRateLimitGroup(description string) string {
	marker := "Rate Limits Group**: `"
	_, after, ok := strings.Cut(description, marker)
	if !ok {
		return ""
	}
	rest := after
	before, _, ok := strings.Cut(rest, "`")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func operationRequiresAccount(op operation) bool {
	for _, p := range op.Parameters {
		if strings.Contains(p.Ref, "AccountSeq") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(p.In), "header") &&
			strings.EqualFold(strings.TrimSpace(p.Name), "X-Tossinvest-Account") {
			return true
		}
	}
	return false
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
