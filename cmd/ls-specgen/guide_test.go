package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testGuide = `{
  "version": "20261109012812600",
  "defaults": {"method": "POST", "fieldRequired": true},
  "servers": [
    {"protocol": "REST", "url": "https://openapi.example:8080/"},
    {"protocol": "WEBSOCKET", "url": "wss://openapi.example:9443", "simulationUrl": "wss://openapi.example:29443"}
  ],
  "apis": [
    {
      "categories": ["주식"],
      "name": "[주식] 시세",
      "description": " 시세 조회 ",
      "protocol": "REST",
      "path": "stock/market-data",
      "transactions": [{
        "code": "t1102",
        "name": "주식현재가(시세)조회",
        "transactionPerSec": 10,
        "request": {
          "headers": [{"element": "content-type", "name": "컨텐츠타입", "type": "String", "length": "100"}],
          "body": [{"element": "t1102InBlock", "name": "t1102InBlock", "type": "Object", "fields": [
            {"element": "shcode", "name": "단축코드", "type": "String", "length": "6"},
            {"element": "exchgubun", "name": "거래소구분코드", "type": "String", "length": "1", "required": false}
          ]}]
        },
        "response": {
          "body": [{"element": "t1102OutBlock", "type": "Object Array", "fields": [
            {"element": "price", "name": "현재가", "type": "Number", "length": "8"}
          ]}]
        }
      }]
    },
    {
      "categories": ["주식"],
      "name": "[주식] 실시간 시세",
      "protocol": "WEBSOCKET",
      "path": "/websocket",
      "commonHeaders": {
        "request": [{"element": "token", "name": "접근토큰", "type": "String", "length": "1000"}],
        "response": [{"element": "tr_cd", "name": "거래 CD", "type": "String", "length": "3"}]
      },
      "transactions": [
        {"code": "s3_", "name": "ELW체결", "request": {}, "response": {}},
        {"code": "S3_", "name": "KOSPI체결", "request": {}, "response": {}}
      ]
    },
    {"categories": ["기타"], "name": "빈 API", "protocol": "REST", "path": "/etc/empty", "transactions": []}
  ]
}`

func decodeTestGuide(t *testing.T, raw string) *guideDocument {
	t.Helper()
	var doc guideDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return &doc
}

func TestSnapshotFromGuide(t *testing.T) {
	snap, err := snapshotFromGuide(decodeTestGuide(t, testGuide))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Endpoints) != 2 {
		t.Fatalf("endpoints = %+v, want REST + WEBSOCKET (APIs without TRs dropped)", snap.Endpoints)
	}

	rest := snap.Endpoints[0]
	if rest.GroupName != "주식" || rest.APIName != "[주식] 시세" || rest.Path != "/stock/market-data" ||
		rest.Method != "POST" || rest.Protocol != "REST" || rest.Domain != "https://openapi.example:8080" ||
		rest.SimulatedDomain != "" || rest.Description != "시세 조회" {
		t.Fatalf("unexpected REST endpoint: %+v", rest)
	}
	tr := rest.TRs[0]
	if tr.Code != "t1102" || tr.Name != "주식현재가(시세)조회" || tr.TransactionPerSec != "10" {
		t.Fatalf("unexpected TR: %+v", tr)
	}
	want := []propertySnapshot{
		{BodyType: "req_h", Code: "content-type", Name: "컨텐츠타입", Type: "A0001", Required: true, Length: "100", Order: "001"},
		{BodyType: "req_b", Code: "t1102InBlock", Name: "t1102InBlock", Type: "A0003", Required: true, Order: "001"},
		{BodyType: "req_b", Code: "shcode", Name: "단축코드", Type: "A0001", Required: true, Length: "6", Order: "001.001"},
		{BodyType: "req_b", Code: "exchgubun", Name: "거래소구분코드", Type: "A0001", Length: "1", Order: "001.002"},
		{BodyType: "res_b", Code: "t1102OutBlock", Type: "A0005", Required: true, Order: "001"},
		{BodyType: "res_b", Code: "price", Name: "현재가", Type: "A0004", Required: true, Length: "8", Order: "001.001"},
	}
	if fmt.Sprint(tr.Properties) != fmt.Sprint(want) {
		t.Fatalf("properties:\n got %+v\nwant %+v", tr.Properties, want)
	}

	ws := snap.Endpoints[1]
	if ws.Protocol != "WEBSOCKET" || ws.Domain != "wss://openapi.example:9443" || ws.SimulatedDomain != "wss://openapi.example:29443" {
		t.Fatalf("unexpected WEBSOCKET endpoint: %+v", ws)
	}
	if len(ws.TRs) != 2 || ws.TRs[0].Code != "S3_" || ws.TRs[1].Code != "s3_" {
		t.Fatalf("case-only TR codes must sort on the exact code: %+v", ws.TRs)
	}
	for _, tr := range ws.TRs {
		if len(tr.Properties) != 2 ||
			tr.Properties[0].BodyType != "req_h" || tr.Properties[0].Code != "token" ||
			tr.Properties[1].BodyType != "res_h" || tr.Properties[1].Code != "tr_cd" {
			t.Fatalf("TR %s did not inherit common headers: %+v", tr.Code, tr.Properties)
		}
	}
}

func TestSnapshotFromGuideDefaultFieldRequired(t *testing.T) {
	guide := strings.Replace(testGuide, `"fieldRequired": true`, `"fieldRequired": false`, 1)
	snap, err := snapshotFromGuide(decodeTestGuide(t, guide))
	if err != nil {
		t.Fatal(err)
	}
	for _, prop := range snap.Endpoints[0].TRs[0].Properties {
		if prop.Required {
			t.Fatalf("%s is required although the guide default is optional", prop.Code)
		}
	}
}

func TestSnapshotFromGuideRejectsUnknownFieldType(t *testing.T) {
	guide := strings.Replace(testGuide, `"type": "Number"`, `"type": "Decimal"`, 1)
	_, err := snapshotFromGuide(decodeTestGuide(t, guide))
	if err == nil || !strings.Contains(err.Error(), `unknown LS field type "Decimal"`) || !strings.Contains(err.Error(), "/stock/market-data t1102") {
		t.Fatalf("err = %v, want unknown type error naming the TR", err)
	}
}

func TestSnapshotFromGuideRejectsEmptyGuide(t *testing.T) {
	if _, err := snapshotFromGuide(decodeTestGuide(t, `{"apis": []}`)); err == nil {
		t.Fatal("empty guide accepted")
	}
}

func TestRefreshWritesSnapshotAndSpecsFromGuide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testGuide)
	}))
	defer srv.Close()
	dir := t.TempDir()
	snapshotPath, specPath := filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "specs.go")
	if err := runRefresh([]string{"--guide-url", srv.URL, "--snapshot", snapshotPath, "--spec-out", specPath}); err != nil {
		t.Fatal(err)
	}
	snap, err := readSnapshot(snapshotPath)
	if err != nil || snap.Source != srv.URL || len(snap.Endpoints) != 2 {
		t.Fatalf("snapshot = %+v, err = %v", snap, err)
	}
	specs, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"/stock/market-data|t1102"`, `"/websocket|s3_"`, `"KOSPI\uccb4\uacb0"`} {
		if !strings.Contains(string(specs), want) {
			t.Fatalf("generated specs missing %s:\n%s", want, specs)
		}
	}
	if strings.Contains(string(specs), `"ELW\uccb4\uacb0"`) {
		t.Fatalf("case-only duplicate s3_ should lose to S3_:\n%s", specs)
	}
	if err := runCheck([]string{"--in", snapshotPath, "--spec-out", specPath}); err != nil {
		t.Fatalf("check after refresh: %v", err)
	}
}

func TestRefreshRejectsNonJSONGuide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html>maintenance</html>")
	}))
	defer srv.Close()
	dir := t.TempDir()
	err := runRefresh([]string{"--guide-url", srv.URL, "--snapshot", filepath.Join(dir, "s.json"), "--spec-out", filepath.Join(dir, "s.go")})
	if err == nil || !strings.Contains(err.Error(), "decode LS API guide") {
		t.Fatalf("err = %v, want decode error", err)
	}
}
