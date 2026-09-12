package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOptionalRequestFieldsSurviveFetchAndGeneration(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("accessUrl"); got != "/uapi/test/order" {
			t.Errorf("accessUrl = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"accessUrl":"/uapi/test/order","httpMethod":"POST","apiPropertys":[
			{"bodyType":"req_b","propertyCd":"CANO","requireYn":"Y","propertyType":"A0001","propertyOrder":"001"},
			{"bodyType":"req_b","propertyCd":"EXCG_ID_DVSN_CD","requireYn":"N","propertyType":"A0001","propertyOrder":"002"},
			{"bodyType":"req_b","propertyCd":"CNDT_PRIC","requireYn":"N","propertyType":"A0001","propertyOrder":"003"}
		]}`)
	}))
	defer server.Close()
	ep, err := fetchEndpointDetail(context.Background(), server.Client(), server.URL, "/uapi/test/order")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ep.RequiredFields, []string{"CANO"}) || len(ep.RequestProps) != 3 {
		t.Fatalf("requiredness lost: %+v", ep)
	}
	snap := &snapshot{Endpoints: []endpointSnapshot{ep}}
	generated, err := generateDocumentedTypesGo(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"json:\"CANO\"", "json:\"EXCG_ID_DVSN_CD,omitempty\"", "json:\"CNDT_PRIC,omitempty\""} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("missing %s in generated request:\n%s", want, generated)
		}
	}
	specs, err := generateDocumentedSpecsGo(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(specs), "EXCG_ID_DVSN_CD") || strings.Contains(string(specs), "CNDT_PRIC") {
		t.Fatalf("optional fields became required:\n%s", specs)
	}
	path := t.TempDir() + "/snapshot.json"
	if err := writeSnapshot(path, snap); err != nil {
		t.Fatal(err)
	}
	reloaded, err := readSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Endpoints[0].RequestProps, ep.RequestProps) {
		t.Fatalf("snapshot round trip lost request properties: %+v", reloaded)
	}
}
