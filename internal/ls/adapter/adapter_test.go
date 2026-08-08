package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	internalls "github.com/smallfish06/krsec/internal/ls"
	"github.com/smallfish06/krsec/pkg/broker"
)

type testTokenManager struct {
	token     string
	expiresAt time.Time
	hasToken  bool
}

func (m *testTokenManager) GetToken(string) (string, time.Time, bool) {
	if m.hasToken {
		return m.token, m.expiresAt, true
	}
	return "", time.Time{}, false
}

func (m *testTokenManager) SetToken(_ string, token string, expiresAt time.Time) error {
	m.token = token
	m.expiresAt = expiresAt
	m.hasToken = true
	return nil
}

func (m *testTokenManager) DeleteToken(string) error {
	m.token = ""
	m.expiresAt = time.Time{}
	m.hasToken = false
	return nil
}

func (m *testTokenManager) WaitForAuth(string) {}

func TestAdapterGetQuote_MapsT1102Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathStockMarket:
			if got := r.Header.Get("tr_cd"); got != internalls.TRStockQuote {
				t.Fatalf("tr_cd = %q, want %s", got, internalls.TRStockQuote)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"t1102OutBlock": map[string]any{
					"shcode":     "078020",
					"price":      "1234",
					"open":       "1200",
					"high":       "1250",
					"low":        "1190",
					"recprice":   "1210",
					"change":     "24",
					"diff":       "1.98",
					"volume":     "3456",
					"value":      "7890",
					"uplmtprice": "1500",
					"dnlmtprice": "900",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	quote, err := a.GetQuote(context.Background(), "KRX", "078020")
	if err != nil {
		t.Fatalf("GetQuote error: %v", err)
	}
	if quote.Symbol != "078020" || quote.Market != "KRX" {
		t.Fatalf("unexpected identity: %+v", quote)
	}
	if quote.Price != 1234 || quote.Open != 1200 || quote.High != 1250 || quote.Low != 1190 {
		t.Fatalf("unexpected OHLC: %+v", quote)
	}
	if quote.PrevClose != 1210 || quote.Change != 24 || quote.ChangeRate != 1.98 {
		t.Fatalf("unexpected change fields: %+v", quote)
	}
	if quote.Volume != 3456 || quote.Turnover != 7890 {
		t.Fatalf("unexpected volume fields: %+v", quote)
	}
}

func TestAdapterGetQuote_MapsOverseasG3101Response(t *testing.T) {
	var gotSymbol string
	var gotExchange string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathOverseasStockMarket:
			if got := r.Header.Get("tr_cd"); got != internalls.TROverseasStockQuote {
				t.Fatalf("tr_cd = %q, want %s", got, internalls.TROverseasStockQuote)
			}
			var body map[string]map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode body: %v", err)
			}
			gotSymbol = body["g3101InBlock"]["symbol"]
			gotExchange = body["g3101InBlock"]["exchcd"]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"g3101OutBlock": map[string]any{
					"delaygb":   "R",
					"keysymbol": "82AAPL",
					"exchcd":    "82",
					"symbol":    "AAPL",
					"korname":   "애플",
					"currency":  "USD",
					"price":     "100.5000",
					"sign":      "5",
					"diff":      "1.2500",
					"rate":      "-1.23",
					"volume":    "12345",
					"amount":    "67890",
					"open":      "101.0000",
					"high":      "102.0000",
					"low":       "99.0000",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	quote, err := a.GetQuote(context.Background(), "NASDAQ", "AAPL")
	if err != nil {
		t.Fatalf("GetQuote error: %v", err)
	}
	if gotSymbol != "AAPL" || gotExchange != "82" {
		t.Fatalf("request symbol/exchange = %q/%q, want AAPL/82", gotSymbol, gotExchange)
	}
	if quote.Symbol != "AAPL" || quote.Market != "US-NASDAQ" {
		t.Fatalf("unexpected identity: %+v", quote)
	}
	if quote.Price != 100.5 || quote.Change != -1.25 || quote.PrevClose != 101.75 || quote.ChangeRate != -1.23 {
		t.Fatalf("unexpected price fields: %+v", quote)
	}
	if quote.Volume != 12345 || quote.Turnover != 67890 {
		t.Fatalf("unexpected volume fields: %+v", quote)
	}
}

func TestAdapterGetQuote_ReturnsErrorWhenG3101OutputMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathOverseasStockMarket:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "해당 자료가 없습니다.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := a.GetQuote(context.Background(), "US-NASDAQ", "AAPL")
	if !errors.Is(err, broker.ErrServerError) {
		t.Fatalf("error = %v, want ErrServerError", err)
	}
	if !strings.Contains(err.Error(), "g3101OutBlock missing") || !strings.Contains(err.Error(), "해당 자료가 없습니다") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterGetOHLCV_UsesHistoricalStartForT8410FromOnly(t *testing.T) {
	var gotLimit float64
	var gotStartDate string
	var gotEndDate string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathStockChart:
			if got := r.Header.Get("tr_cd"); got != internalls.TRStockChart {
				t.Fatalf("tr_cd = %q, want %s", got, internalls.TRStockChart)
			}
			var payload map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			block := payload["t8410InBlock"]
			gotLimit, _ = block["qrycnt"].(float64)
			gotStartDate, _ = block["sdate"].(string)
			gotEndDate, _ = block["edate"].(string)
			if block["cts_date"] != "" || block["comp_yn"] != "N" || block["sujung"] != "Y" {
				t.Fatalf("unexpected chart block: %#v", block)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"t8410OutBlock1": []map[string]any{
					{"date": "20250203", "open": "1000", "high": "1100", "low": "990", "close": "1080", "jdiff_vol": "10000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	rows, err := a.GetOHLCV(context.Background(), "KRX", "078020", broker.OHLCVOpts{
		Interval: "1d",
		From:     time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC),
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("GetOHLCV error: %v", err)
	}
	if gotLimit != 5 {
		t.Fatalf("qrycnt = %v, want 5", gotLimit)
	}
	if gotStartDate != "20250203" {
		t.Fatalf("sdate = %q, want 20250203", gotStartDate)
	}
	if gotEndDate != "20250207" {
		t.Fatalf("edate = %q, want 20250207 for from-only historical request", gotEndDate)
	}
	if len(rows) != 1 || rows[0].Timestamp.Format("20060102") != "20250203" || rows[0].Volume != 10000 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestAdapterGetOHLCV_UsesOpenEndedEndForT8410Latest(t *testing.T) {
	var gotStartDate string
	var gotEndDate string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathStockChart:
			var payload map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			block := payload["t8410InBlock"]
			gotStartDate, _ = block["sdate"].(string)
			gotEndDate, _ = block["edate"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"t8410OutBlock1": []map[string]any{
					{"date": "20260601", "open": "1000", "high": "1100", "low": "990", "close": "1080", "jdiff_vol": "10000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	rows, err := a.GetOHLCV(context.Background(), "KRX", "078020", broker.OHLCVOpts{Interval: "1d", Limit: 5})
	if err != nil {
		t.Fatalf("GetOHLCV error: %v", err)
	}
	if gotStartDate != "" {
		t.Fatalf("sdate = %q, want empty string for latest request", gotStartDate)
	}
	if gotEndDate != "99999999" {
		t.Fatalf("edate = %q, want 99999999 for latest request", gotEndDate)
	}
	if len(rows) != 1 || rows[0].Timestamp.Format("20060102") != "20260601" || rows[0].Volume != 10000 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestAdapterGetOHLCV_MapsOverseasG3204Response(t *testing.T) {
	var gotLimit float64
	var gotSymbol string
	var gotEndDate string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathOverseasStockChart:
			if got := r.Header.Get("tr_cd"); got != internalls.TROverseasStockChart {
				t.Fatalf("tr_cd = %q, want %s", got, internalls.TROverseasStockChart)
			}
			var body map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode body: %v", err)
			}
			gotSymbol, _ = body["g3204InBlock"]["symbol"].(string)
			gotLimit, _ = body["g3204InBlock"]["qrycnt"].(float64)
			gotEndDate, _ = body["g3204InBlock"]["edate"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"g3204OutBlock1": []map[string]any{
					{"date": "20250601", "open": "100.0", "high": "110.0", "low": "99.0", "close": "108.0", "volume": "1000"},
					{"date": "20250602", "open": "108.0", "high": "112.0", "low": "107.0", "close": "109.0", "volume": "2000"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	rows, err := a.GetOHLCV(context.Background(), "US-NASDAQ", "AAPL", broker.OHLCVOpts{Interval: "1d", Limit: 10})
	if err != nil {
		t.Fatalf("GetOHLCV error: %v", err)
	}
	if gotSymbol != "AAPL" {
		t.Fatalf("request symbol = %q, want AAPL", gotSymbol)
	}
	if gotLimit != 5 {
		t.Fatalf("request qrycnt = %v, want capped 5", gotLimit)
	}
	if gotEndDate != "" {
		t.Fatalf("request edate = %q, want empty string", gotEndDate)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Close != 108 || rows[1].Volume != 2000 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestAdapterGetOHLCV_ReturnsErrorWhenG3204OutputMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathOverseasStockChart:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "해당 자료가 없습니다.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := a.GetOHLCV(context.Background(), "US-NASDAQ", "AAPL", broker.OHLCVOpts{Interval: "1d", Limit: 5})
	if !errors.Is(err, broker.ErrServerError) {
		t.Fatalf("error = %v, want ErrServerError", err)
	}
	if !strings.Contains(err.Error(), "g3204OutBlock1 missing") || !strings.Contains(err.Error(), "해당 자료가 없습니다") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterGetInstrument_MapsOverseasG3104Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case internalls.PathOAuthToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case internalls.PathOverseasStockMarket:
			if got := r.Header.Get("tr_cd"); got != internalls.TROverseasStockInstrument {
				t.Fatalf("tr_cd = %q, want %s", got, internalls.TROverseasStockInstrument)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd":  "00000",
				"rsp_msg": "ok",
				"g3104OutBlock": map[string]any{
					"keysymbol":     "82AAPL",
					"exchcd":        "82",
					"symbol":        "AAPL",
					"korname":       "애플",
					"engname":       "APPLE INC",
					"exchange_name": "나스닥",
					"nation_name":   "미국",
					"induname":      "Technology Hardware",
					"instname":      "주식",
					"currency":      "USD",
					"suspend":       "N",
					"share":         "15000000000",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "ls-main", &testTokenManager{}, "", nil)
	a.Client().SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "app-key", AppSecret: "app-secret"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	inst, err := a.GetInstrument(context.Background(), "US-NASDAQ", "AAPL")
	if err != nil {
		t.Fatalf("GetInstrument error: %v", err)
	}
	if inst.Symbol != "AAPL" || inst.Market != "US-NASDAQ" || inst.Currency != "USD" {
		t.Fatalf("unexpected identity: %+v", inst)
	}
	if inst.Name != "애플" || inst.NameEn != "APPLE INC" || inst.AssetType != broker.AssetOverseas {
		t.Fatalf("unexpected metadata: %+v", inst)
	}
	if inst.IsSuspended {
		t.Fatalf("instrument should not be suspended: %+v", inst)
	}
}
