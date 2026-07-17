package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultListURL      = "https://openapi.kiwoom.com/guide/getApiInfoListAjax"
	defaultSnapshotPath = "pkg/kiwoom/specs/documented_endpoints.json"
	defaultSpecsOutPath = "pkg/kiwoom/specs/documented_specs_generated.go"
	defaultTypesOutPath = "pkg/kiwoom/specs/documented_endpoint_types_generated.go"
)

type snapshot struct {
	Endpoints []endpointSnapshot `json:"endpoints"`
}

type endpointSnapshot struct {
	Path           string                 `json:"path"`
	APIID          string                 `json:"api_id"`
	Method         string                 `json:"method"`
	RequiredFields []string               `json:"required_fields,omitempty"`
	RequestFields  []requestFieldSnapshot `json:"request_fields,omitempty"`
	ResponseFields []requestFieldSnapshot `json:"response_fields,omitempty"`
}

type requestFieldSnapshot struct {
	Code     string `json:"code"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type apiListResponse struct {
	RespCode string     `json:"resp_code"`
	RespMsg  string     `json:"resp_msg"`
	RespData []apiEntry `json:"resp_data"`
}

type apiEntry struct {
	APIInfo apiInfo      `json:"apiInfo"`
	APITrIO []apiTRIORow `json:"apiTrIo"`
}

type apiInfo struct {
	SVCURI    string `json:"svcUri"`
	APIID     string `json:"apiId"`
	JobMethod string `json:"jobMethod"`
	SvcTrans  string `json:"svcTransTp"`
}

type apiTRIORow struct {
	ItemID       string `json:"itemId"`
	InputOutput  string `json:"inptOutputTp"`
	Essential    string `json:"esntYn"`
	Type         string `json:"type"`
	TypeNm       string `json:"typeNm"`
	ItemType     string `json:"itemType"`
	HeaderBodyTp string `json:"headBodyTp"`
}

// fieldType normalizes the documented field type across portal schema
// revisions: older payloads used type="LIST"/"String", newer ones use
// typeNm="List<Map>"/"String" plus itemType="R" for repeated groups.
func (r apiTRIORow) fieldType() string {
	t := strings.TrimSpace(r.Type)
	if t == "" {
		t = strings.TrimSpace(r.TypeNm)
	}
	if strings.EqualFold(strings.TrimSpace(r.ItemType), "R") || strings.HasPrefix(strings.ToLower(t), "list") {
		return "LIST"
	}
	return t
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
		fmt.Fprintf(os.Stderr, "kiwoom-specgen: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  kiwoom-specgen fetch [flags]      # fetch snapshot from Kiwoom docs endpoint")
	fmt.Fprintln(os.Stderr, "  kiwoom-specgen generate [flags]   # generate Go files from snapshot")
	fmt.Fprintln(os.Stderr, "  kiwoom-specgen refresh [flags]    # fetch + generate")
	fmt.Fprintln(os.Stderr, "  kiwoom-specgen check [flags]      # verify generated files are up to date")
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	listURL := fs.String("list-url", defaultListURL, "Kiwoom API list URL")
	out := fs.String("out", defaultSnapshotPath, "snapshot JSON output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *listURL)
	if err != nil {
		return err
	}
	return writeSnapshot(*out, snap)
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "generated spec Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "generated request/response type Go output path")
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
	typeBytes, err := generateDocumentedTypesGo(snap)
	if err != nil {
		return err
	}

	if err := writeFile(*specOut, specBytes); err != nil {
		return err
	}
	return writeFile(*typesOut, typeBytes)
}

func runRefresh(args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	listURL := fs.String("list-url", defaultListURL, "Kiwoom API list URL")
	snapshotPath := fs.String("snapshot", defaultSnapshotPath, "snapshot JSON path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "generated spec Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "generated request/response type Go output path")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	snap, err := fetchSnapshot(client, *listURL)
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
	typeBytes, err := generateDocumentedTypesGo(snap)
	if err != nil {
		return err
	}
	if err := writeFile(*specOut, specBytes); err != nil {
		return err
	}
	return writeFile(*typesOut, typeBytes)
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	in := fs.String("in", defaultSnapshotPath, "snapshot JSON input path")
	specOut := fs.String("spec-out", defaultSpecsOutPath, "generated spec Go output path")
	typesOut := fs.String("types-out", defaultTypesOutPath, "generated request/response type Go output path")
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
		return fmt.Errorf("%w (run: go run ./cmd/kiwoom-specgen generate)", err)
	}
	if err := compareGenerated(*typesOut, wantTypes); err != nil {
		return fmt.Errorf("%w (run: go run ./cmd/kiwoom-specgen generate)", err)
	}
	return nil
}

func fetchSnapshot(client *http.Client, listURL string) (*snapshot, error) {
	form := url.Values{}
	form.Set("apiId", "")

	req, err := http.NewRequest(http.MethodPost, listURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "krsec-kiwoom-specgen/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch API list HTTP %d", resp.StatusCode)
	}

	var payload apiListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode API list response: %w", err)
	}
	if strings.TrimSpace(payload.RespCode) != "0" {
		return nil, fmt.Errorf("API list response error: code=%s msg=%s", strings.TrimSpace(payload.RespCode), strings.TrimSpace(payload.RespMsg))
	}

	byKey := make(map[string]endpointSnapshot, len(payload.RespData))
	for _, row := range payload.RespData {
		path := strings.TrimSpace(row.APIInfo.SVCURI)
		apiID := strings.ToLower(strings.TrimSpace(row.APIInfo.APIID))
		method := strings.ToUpper(strings.TrimSpace(row.APIInfo.JobMethod))
		svcTrans := strings.ToUpper(strings.TrimSpace(row.APIInfo.SvcTrans))

		if path == "" || apiID == "" {
			continue
		}
		if svcTrans != "REST" {
			continue
		}
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		if path == "/api/dostk/websocket" {
			continue
		}
		if method == "" {
			method = http.MethodPost
		}

		requestFields := extractRequestFields(row.APITrIO)
		responseFields := extractResponseFields(row.APITrIO)
		requiredFields := collectRequiredFields(requestFields)

		key := documentedEndpointKey(path, apiID)
		ep := endpointSnapshot{
			Path:           path,
			APIID:          apiID,
			Method:         method,
			RequiredFields: requiredFields,
			RequestFields:  requestFields,
			ResponseFields: responseFields,
		}
		if existing, ok := byKey[key]; ok {
			ep = mergeEndpointSnapshot(existing, ep)
		}
		byKey[key] = ep
	}

	if len(byKey) == 0 {
		return nil, errors.New("no /api REST endpoints discovered from Kiwoom API list")
	}

	endpoints := make([]endpointSnapshot, 0, len(byKey))
	for _, ep := range byKey {
		endpoints = append(endpoints, ep)
	}
	canonicalizeEndpoints(endpoints)
	return &snapshot{Endpoints: endpoints}, nil
}

func extractRequestFields(rows []apiTRIORow) []requestFieldSnapshot {
	return extractIOFields(rows, "I")
}

func extractResponseFields(rows []apiTRIORow) []requestFieldSnapshot {
	return extractIOFields(rows, "O")
}

func extractIOFields(rows []apiTRIORow, ioType string) []requestFieldSnapshot {
	ioType = strings.ToUpper(strings.TrimSpace(ioType))
	byCode := map[string]requestFieldSnapshot{}
	for _, row := range rows {
		if strings.ToUpper(strings.TrimSpace(row.InputOutput)) != ioType {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(row.HeaderBodyTp)) != "B" {
			continue
		}
		code := strings.TrimSpace(row.ItemID)
		if code == "" {
			continue
		}
		code = strings.ToLower(code)
		field := requestFieldSnapshot{
			Code:     code,
			Type:     row.fieldType(),
			Required: strings.EqualFold(strings.TrimSpace(row.Essential), "Y"),
		}
		if existing, ok := byCode[code]; ok {
			if existing.Type == "" {
				existing.Type = field.Type
			}
			existing.Required = existing.Required || field.Required
			byCode[code] = existing
			continue
		}
		byCode[code] = field
	}
	out := make([]requestFieldSnapshot, 0, len(byCode))
	for _, field := range byCode {
		out = append(out, field)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func collectRequiredFields(fields []requestFieldSnapshot) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if !field.Required {
			continue
		}
		out = append(out, field.Code)
	}
	sort.Strings(out)
	return out
}

func mergeEndpointSnapshot(a, b endpointSnapshot) endpointSnapshot {
	out := endpointSnapshot{
		Path:   firstNonEmpty(a.Path, b.Path),
		APIID:  firstNonEmpty(a.APIID, b.APIID),
		Method: firstNonEmpty(a.Method, b.Method),
	}
	out.RequestFields = mergeFieldSnapshots(a.RequestFields, b.RequestFields)
	out.ResponseFields = mergeFieldSnapshots(a.ResponseFields, b.ResponseFields)
	out.RequiredFields = collectRequiredFields(out.RequestFields)
	return out
}

func mergeFieldSnapshots(a, b []requestFieldSnapshot) []requestFieldSnapshot {
	byCode := make(map[string]requestFieldSnapshot, len(a)+len(b))
	for _, fields := range [][]requestFieldSnapshot{a, b} {
		for _, field := range fields {
			code := strings.ToLower(strings.TrimSpace(field.Code))
			if code == "" {
				continue
			}
			field.Code = code
			if existing, ok := byCode[code]; ok {
				if existing.Type == "" {
					existing.Type = field.Type
				}
				existing.Required = existing.Required || field.Required
				byCode[code] = existing
				continue
			}
			byCode[code] = field
		}
	}
	out := make([]requestFieldSnapshot, 0, len(byCode))
	for _, field := range byCode {
		out = append(out, field)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
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

	canonicalizeEndpoints(snap.Endpoints)
	return &snap, nil
}

func canonicalizeEndpoints(endpoints []endpointSnapshot) {
	for i := range endpoints {
		ep := &endpoints[i]
		ep.Path = strings.TrimSpace(ep.Path)
		ep.APIID = strings.ToLower(strings.TrimSpace(ep.APIID))
		ep.Method = strings.ToUpper(strings.TrimSpace(ep.Method))
		if ep.Method == "" {
			ep.Method = http.MethodPost
		}
		ep.RequestFields = canonicalizeRequestFields(ep.RequestFields)
		ep.ResponseFields = canonicalizeRequestFields(ep.ResponseFields)
		ep.RequiredFields = collectRequiredFields(ep.RequestFields)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].APIID < endpoints[j].APIID
	})
}

func canonicalizeRequestFields(fields []requestFieldSnapshot) []requestFieldSnapshot {
	byCode := map[string]requestFieldSnapshot{}
	for _, field := range fields {
		code := strings.ToLower(strings.TrimSpace(field.Code))
		if code == "" {
			continue
		}
		normalized := requestFieldSnapshot{
			Code:     code,
			Type:     strings.TrimSpace(field.Type),
			Required: field.Required,
		}
		if existing, ok := byCode[code]; ok {
			if existing.Type == "" {
				existing.Type = normalized.Type
			}
			existing.Required = existing.Required || normalized.Required
			byCode[code] = existing
			continue
		}
		byCode[code] = normalized
	}
	out := make([]requestFieldSnapshot, 0, len(byCode))
	for _, field := range byCode {
		out = append(out, field)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func writeSnapshot(path string, snap *snapshot) error {
	if snap == nil {
		return errors.New("snapshot is nil")
	}
	canonicalizeEndpoints(snap.Endpoints)

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
	apiIDs := make([]string, 0, len(snap.Endpoints))
	seenAPIIDs := make(map[string]struct{}, len(snap.Endpoints))
	for _, ep := range snap.Endpoints {
		apiID := strings.ToLower(strings.TrimSpace(ep.APIID))
		if apiID == "" {
			continue
		}
		if _, exists := seenAPIIDs[apiID]; exists {
			continue
		}
		seenAPIIDs[apiID] = struct{}{}
		apiIDs = append(apiIDs, apiID)
	}
	sort.Strings(apiIDs)

	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/kiwoom-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/kiwoom/specs/documented_endpoints.json\n\n")
	b.WriteString("// Documented Kiwoom API IDs generated from snapshot.\n")
	b.WriteString("const (\n")
	usedConstNames := make(map[string]int, len(apiIDs))
	for _, apiID := range apiIDs {
		name := "KiwoomAPIID" + sanitizeIdentifier(apiID)
		if name == "KiwoomAPIID" {
			name = "KiwoomAPIIDUnknown"
		}
		if n := usedConstNames[name]; n > 0 {
			usedConstNames[name] = n + 1
			name = fmt.Sprintf("%s%d", name, n+1)
		} else {
			usedConstNames[name] = 1
		}
		fmt.Fprintf(&b, "\t%s = %q\n", name, apiID)
	}
	b.WriteString(")\n\n")
	b.WriteString("// KiwoomRequestFieldSpec defines one documented endpoint field.\n")
	b.WriteString("type KiwoomRequestFieldSpec struct {\n")
	b.WriteString("\tCode     string\n")
	b.WriteString("\tType     string\n")
	b.WriteString("\tRequired bool\n")
	b.WriteString("}\n\n")
	b.WriteString("// KiwoomEndpointSpec defines one Kiwoom endpoint specification from official docs.\n")
	b.WriteString("type KiwoomEndpointSpec struct {\n")
	b.WriteString("\tPath           string\n")
	b.WriteString("\tAPIID          string\n")
	b.WriteString("\tMethod         string\n")
	b.WriteString("\tRequiredFields []string\n")
	b.WriteString("\tRequestFields  []KiwoomRequestFieldSpec\n")
	b.WriteString("\tResponseFields []KiwoomRequestFieldSpec\n")
	b.WriteString("}\n\n")
	b.WriteString("// DocumentedKiwoomEndpointSpecs is generated from documented Kiwoom snapshot.\n")
	b.WriteString("// Key format: <path>|<lower(api_id)>.\n")
	b.WriteString("var DocumentedKiwoomEndpointSpecs = map[string]KiwoomEndpointSpec{\n")
	for _, ep := range snap.Endpoints {
		key := documentedEndpointKey(ep.Path, ep.APIID)
		fmt.Fprintf(&b, "\t%q: {Path: %q, APIID: %q, Method: %q, RequiredFields: []string{", key, ep.Path, ep.APIID, ep.Method)
		for i, code := range ep.RequiredFields {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", code)
		}
		b.WriteString("}, RequestFields: []KiwoomRequestFieldSpec{")
		for _, field := range ep.RequestFields {
			fmt.Fprintf(&b, "{Code: %q, Type: %q, Required: %t}, ", field.Code, field.Type, field.Required)
		}
		b.WriteString("}, ResponseFields: []KiwoomRequestFieldSpec{")
		for _, field := range ep.ResponseFields {
			fmt.Fprintf(&b, "{Code: %q, Type: %q, Required: %t}, ", field.Code, field.Type, field.Required)
		}
		b.WriteString("}},\n")
	}
	b.WriteString("}\n")

	return format.Source([]byte(b.String()))
}

type endpointModel struct {
	Key            string
	RequestType    string
	RequestFields  []requestFieldSnapshot
	ResponseType   string
	ResponseFields []requestFieldSnapshot
	Path           string
	APIID          string
	Method         string
	ReqList        []string
}

func generateDocumentedTypesGo(snap *snapshot) ([]byte, error) {
	models := make([]endpointModel, 0, len(snap.Endpoints))
	needJSONImport := false
	for _, ep := range snap.Endpoints {
		for _, field := range ep.ResponseFields {
			if isListFieldType(field.Type) {
				needJSONImport = true
				break
			}
		}
		models = append(models, endpointModel{
			Key:            documentedEndpointKey(ep.Path, ep.APIID),
			RequestType:    requestTypeName(ep),
			RequestFields:  append([]requestFieldSnapshot(nil), ep.RequestFields...),
			ResponseType:   responseTypeName(ep),
			ResponseFields: append([]requestFieldSnapshot(nil), ep.ResponseFields...),
			Path:           ep.Path,
			APIID:          ep.APIID,
			Method:         ep.Method,
			ReqList:        append([]string(nil), ep.RequiredFields...),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Key < models[j].Key })

	var b strings.Builder
	b.WriteString("//nolint:all // Generated code; source schema can include non-standard tags/words.\n")
	b.WriteString("package specs\n\n")
	b.WriteString("// Code generated by cmd/kiwoom-specgen. DO NOT EDIT.\n")
	b.WriteString("// Source: pkg/kiwoom/specs/documented_endpoints.json\n\n")
	if needJSONImport {
		b.WriteString("import \"encoding/json\"\n\n")
	}

	for _, m := range models {
		emitEndpointRequestType(&b, m)
	}
	for _, m := range models {
		emitEndpointResponseType(&b, m)
	}

	b.WriteString("var documentedEndpointRequestFactories = map[string]func() any{\n")
	for _, m := range models {
		fmt.Fprintf(&b, "\t%q: func() any { return &%s{} },\n", m.Key, m.RequestType)
	}
	b.WriteString("}\n")
	b.WriteString("\n")
	b.WriteString("var documentedEndpointResponseFactories = map[string]func() any{\n")
	for _, m := range models {
		fmt.Fprintf(&b, "\t%q: func() any { return &%s{} },\n", m.Key, m.ResponseType)
	}
	b.WriteString("}\n")

	return format.Source([]byte(b.String()))
}

func emitEndpointRequestType(b *strings.Builder, m endpointModel) {
	fmt.Fprintf(b, "type %s struct {\n", m.RequestType)
	if len(m.RequestFields) == 0 {
		b.WriteString("}\n\n")
		return
	}
	used := map[string]int{}
	for _, field := range m.RequestFields {
		goName := goFieldName(field.Code, used)
		fmt.Fprintf(b, "\t%s string `json:\"%s,omitempty\"`\n", goName, field.Code)
	}
	b.WriteString("}\n\n")
}

func emitEndpointResponseType(b *strings.Builder, m endpointModel) {
	if len(m.ResponseFields) == 0 {
		fmt.Fprintf(b, "type %s struct {\n", m.ResponseType)
		b.WriteString("}\n\n")
		return
	}

	hasListField := false
	for _, field := range m.ResponseFields {
		if isListFieldType(field.Type) {
			hasListField = true
			break
		}
	}

	// Kiwoom docs encode LIST item fields with a leading "- " marker.
	// Keep those fields in a nested item type instead of top-level response.
	topFields := make([]requestFieldSnapshot, 0, len(m.ResponseFields))
	itemFields := make([]requestFieldSnapshot, 0)
	for _, field := range m.ResponseFields {
		if hasListField && isKiwoomListItemField(field.Code) {
			itemFields = append(itemFields, field)
			continue
		}
		topFields = append(topFields, field)
	}

	itemListCode := ""
	itemTypeName := ""
	if len(itemFields) > 0 {
		itemListCode = preferredListFieldCode(topFields)
		if itemListCode != "" {
			itemTypeName = m.ResponseType + "Item"
		}
	}

	fmt.Fprintf(b, "type %s struct {\n", m.ResponseType)
	used := map[string]int{}
	for _, field := range topFields {
		jsonCode := normalizeKiwoomFieldCode(field.Code)
		if jsonCode == "" {
			continue
		}
		goName := goFieldName(jsonCode, used)
		goType := responseFieldGoType(field)
		if itemTypeName != "" && isListFieldType(field.Type) && normalizeKiwoomFieldCode(field.Code) == itemListCode {
			goType = "[]" + itemTypeName
		}
		fmt.Fprintf(b, "\t%s %s `json:\"%s,omitempty\"`\n", goName, goType, jsonCode)
	}
	b.WriteString("}\n\n")

	if itemTypeName == "" {
		return
	}

	fmt.Fprintf(b, "type %s struct {\n", itemTypeName)
	itemUsed := map[string]int{}
	for _, field := range itemFields {
		jsonCode := normalizeKiwoomFieldCode(field.Code)
		if jsonCode == "" {
			continue
		}
		goName := goFieldName(jsonCode, itemUsed)
		fmt.Fprintf(b, "\t%s %s `json:\"%s,omitempty\"`\n", goName, responseFieldGoType(field), jsonCode)
	}
	b.WriteString("}\n\n")
}

func responseFieldGoType(field requestFieldSnapshot) string {
	if isListFieldType(field.Type) {
		return "json.RawMessage"
	}
	return "string"
}

func isListFieldType(t string) bool {
	return strings.EqualFold(strings.TrimSpace(t), "LIST")
}

func isKiwoomListItemField(code string) bool {
	return strings.HasPrefix(strings.TrimSpace(code), "-")
}

func normalizeKiwoomFieldCode(code string) string {
	trimmed := strings.TrimSpace(code)
	trimmed = strings.TrimPrefix(trimmed, "-")
	return strings.TrimSpace(trimmed)
}

func preferredListFieldCode(fields []requestFieldSnapshot) string {
	for _, field := range fields {
		if !isListFieldType(field.Type) {
			continue
		}
		if isKiwoomListItemField(field.Code) {
			continue
		}
		if code := normalizeKiwoomFieldCode(field.Code); code != "" {
			return code
		}
	}
	for _, field := range fields {
		if !isListFieldType(field.Type) {
			continue
		}
		if code := normalizeKiwoomFieldCode(field.Code); code != "" {
			return code
		}
	}
	return ""
}

func requestTypeName(ep endpointSnapshot) string {
	pathToken := sanitizeIdentifier(strings.Trim(ep.Path, "/"))
	apiToken := sanitizeIdentifier(ep.APIID)
	if pathToken == "" {
		pathToken = "Endpoint"
	}
	if apiToken == "" {
		apiToken = "Api"
	}
	return "Kiwoom" + pathToken + apiToken + "Request"
}

func responseTypeName(ep endpointSnapshot) string {
	pathToken := sanitizeIdentifier(strings.Trim(ep.Path, "/"))
	apiToken := sanitizeIdentifier(ep.APIID)
	if pathToken == "" {
		pathToken = "Endpoint"
	}
	if apiToken == "" {
		apiToken = "Api"
	}
	return "Kiwoom" + pathToken + apiToken + "Response"
}

func goFieldName(code string, used map[string]int) string {
	base := sanitizeIdentifier(code)
	if base == "" {
		base = "Field"
	}
	if unicode.IsDigit(rune(base[0])) {
		base = "X" + base
	}
	if isGoKeyword(strings.ToLower(base)) {
		base += "Field"
	}
	if n := used[base]; n > 0 {
		used[base] = n + 1
		return fmt.Sprintf("%s%d", base, n+1)
	}
	used[base] = 1
	return base
}

func sanitizeIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

func isGoKeyword(s string) bool {
	switch s {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}

func documentedEndpointKey(path, apiID string) string {
	return strings.TrimSpace(path) + "|" + strings.ToLower(strings.TrimSpace(apiID))
}
