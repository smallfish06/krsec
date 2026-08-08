package kis

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDomesticMSTRecords(t *testing.T) {
	t.Parallel()

	part1 := "A005930  KR7005930003SAMSUNG ELECTRONICS"
	var tail strings.Builder
	for range 228 {
		tail.WriteString(" ")
	}
	raw := []byte(part1 + tail.String() + "\n")

	recs := parseDomesticMSTRecords(raw, 228, "KOSPI", "KRX")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Symbol != "005930" {
		t.Fatalf("unexpected symbol: %s", recs[0].Symbol)
	}
	if recs[0].Market != "KOSPI" {
		t.Fatalf("unexpected market: %s", recs[0].Market)
	}
}

func TestParseOverseasCODRecords(t *testing.T) {
	t.Parallel()

	line := "US\tIDX\tNASD\tNASDAQ\tAAPL\tDNASAAPL\t애플\tApple Inc\t2\tUSD"
	raw := []byte(line + "\n")

	recs := parseOverseasCODRecords(raw, "US-NASDAQ", "NASD", "US")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Symbol != "AAPL" {
		t.Fatalf("unexpected symbol: %s", recs[0].Symbol)
	}
	if recs[0].Exchange != "NASD" {
		t.Fatalf("unexpected exchange: %s", recs[0].Exchange)
	}
	if recs[0].ProductTypeCode != "512" {
		t.Fatalf("unexpected product type code: %s", recs[0].ProductTypeCode)
	}
}

func TestLookupMasterSymbolFromIndex(t *testing.T) {
	t.Parallel()

	idx := masterSymbolsIndex{
		byMarketSymbol: map[string]MasterSymbol{},
		domesticBySym:  map[string]MasterSymbol{},
		usBySym:        map[string]MasterSymbol{},
		anyBySym:       map[string]MasterSymbol{},
	}
	idx.add(MasterSymbol{Symbol: "005930", Market: "KOSPI", Name: "삼성전자", Exchange: "KRX", Currency: "KRW", Country: "KR", ProductTypeCode: "300", IsListed: true})
	idx.add(MasterSymbol{Symbol: "AAPL", Market: "US-NASDAQ", Name: "Apple", Exchange: "NASD", Currency: "USD", Country: "US", ProductTypeCode: "512", IsListed: true})

	if rec, ok := lookupMasterSymbolFromIndex(idx, "KRX", "005930"); !ok || rec.Symbol != "005930" {
		t.Fatalf("expected domestic symbol lookup success")
	}
	if rec, ok := lookupMasterSymbolFromIndex(idx, "US", "AAPL"); !ok || rec.Symbol != "AAPL" {
		t.Fatalf("expected US symbol lookup success")
	}
	if _, ok := lookupMasterSymbolFromIndex(idx, "KRX", "000000"); ok {
		t.Fatalf("expected not found")
	}
}

func TestBootstrapMasterSymbols_RetryAfterFailure(t *testing.T) {
	origDomestic := domesticMasterSources
	origOverseas := overseasMasterSources
	defer func() {
		domesticMasterSources = origDomestic
		overseasMasterSources = origOverseas
		resetMasterSymbolsStateForTest()
	}()

	resetMasterSymbolsStateForTest()
	overseasMasterSources = nil

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer failServer.Close()
	domesticMasterSources = []domesticMasterSource{
		{URL: failServer.URL + "/kospi_code.mst.zip", Market: "KOSPI", Exchange: "KRX", TailLen: 228},
	}

	client := NewClientWithTokenManager(false, nil)
	if _, err := client.BootstrapMasterSymbols(context.Background()); err == nil {
		t.Fatalf("expected bootstrap failure")
	}

	successPayload := buildTestMasterZip(t, "kospi_code.mst", "A005930  KR7005930003SAMSUNG ELECTRONICS"+spaces(228)+"\n")
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(successPayload)
	}))
	defer successServer.Close()
	domesticMasterSources = []domesticMasterSource{
		{URL: successServer.URL + "/kospi_code.mst.zip", Market: "KOSPI", Exchange: "KRX", TailLen: 228},
	}

	count, err := client.BootstrapMasterSymbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected bootstrap retry error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if rec, ok := LookupMasterSymbol("KRX", "005930"); !ok || rec.Symbol != "005930" {
		t.Fatalf("expected cached symbol after retry")
	}
}

func TestReloadMasterSymbols_OverridesExistingCache(t *testing.T) {
	origDomestic := domesticMasterSources
	origOverseas := overseasMasterSources
	defer func() {
		domesticMasterSources = origDomestic
		overseasMasterSources = origOverseas
		resetMasterSymbolsStateForTest()
	}()

	resetMasterSymbolsStateForTest()
	overseasMasterSources = nil

	firstPayload := buildTestMasterZip(t, "kospi_code.mst", "A005930  KR7005930003SAMSUNG ELECTRONICS"+spaces(228)+"\n")
	secondPayload := buildTestMasterZip(t, "kospi_code.mst", "A000660  KR7000660001SK HYNIX"+spaces(228)+"\n")

	payload := firstPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	domesticMasterSources = []domesticMasterSource{
		{URL: server.URL + "/kospi_code.mst.zip", Market: "KOSPI", Exchange: "KRX", TailLen: 228},
	}

	client := NewClientWithTokenManager(false, nil)
	if _, err := client.BootstrapMasterSymbols(context.Background()); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if _, ok := LookupMasterSymbol("KRX", "005930"); !ok {
		t.Fatalf("expected first symbol")
	}

	payload = secondPayload
	if _, err := client.ReloadMasterSymbols(context.Background()); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if _, ok := LookupMasterSymbol("KRX", "000660"); !ok {
		t.Fatalf("expected second symbol after reload")
	}
}

func buildTestMasterZip(t *testing.T, fileName, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(fileName)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func resetMasterSymbolsStateForTest() {
	masterSymbolsMu.Lock()
	defer masterSymbolsMu.Unlock()

	masterSymbolsBootstrapping = false
	masterSymbolsLoaded = false
	masterSymbolsCount = 0
	masterSymbols = masterSymbolsIndex{
		byMarketSymbol: make(map[string]MasterSymbol),
		domesticBySym:  make(map[string]MasterSymbol),
		usBySym:        make(map[string]MasterSymbol),
		anyBySym:       make(map[string]MasterSymbol),
	}
}

func spaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
