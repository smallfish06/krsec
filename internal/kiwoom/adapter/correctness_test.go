package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

func correctionTestAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a := NewAdapterWithOptions(false, "test", &testTokenManager{token: "test-token", expiresAt: time.Now().Add(time.Hour), hasToken: true}, t.TempDir(), nil)
	a.client.SetBaseURL(server.URL)
	a.client.SetCredentials("test-key", "test-secret")
	return a
}

func TestMarketDataVenueRouting(t *testing.T) {
	for _, test := range []struct{ market, symbol, want, output string }{
		{"KRX", "005930", "005930", "KRX"}, {"NXT", "005930", "005930_NX", "NXT"},
		{"SOR", "A005930", "005930_AL", "SOR"}, {"NXT", "005930_NX", "005930_NX", "NXT"},
		{"", "005930_AL", "005930_AL", "SOR"},
	} {
		t.Run(test.market+test.symbol, func(t *testing.T) {
			a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["stk_cd"] != test.want {
					t.Errorf("stk_cd=%v, want %s", body["stk_cd"], test.want)
				}
				response := map[string]any{"return_code": 0, "stk_cd": test.want, "cur_prc": "70000"}
				switch r.Header.Get("api-id") {
				case "ka10081":
					response["stk_dt_pole_chart_qry"] = []map[string]string{{"dt": "20260911", "cur_prc": "70000"}}
				case "ka10082":
					response["stk_stk_pole_chart_qry"] = []map[string]string{{"dt": "20260911", "cur_prc": "70000"}}
				case "ka10083":
					response["stk_mth_pole_chart_qry"] = []map[string]string{{"dt": "20260911", "cur_prc": "70000"}}
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			quote, err := a.GetQuote(context.Background(), test.market, test.symbol)
			if err != nil {
				t.Fatal(err)
			}
			if quote.Symbol != "005930" || quote.Market != test.output {
				t.Fatalf("quote=%+v", quote)
			}
			for _, interval := range []string{"1d", "1w", "1mo"} {
				rows, err := a.GetOHLCV(context.Background(), test.market, test.symbol, broker.OHLCVOpts{Interval: interval})
				if err != nil || len(rows) != 1 {
					t.Fatalf("%s rows=%+v err=%v", interval, rows, err)
				}
			}
		})
	}
}

func TestMarketDataRejectsConflictingSuffix(t *testing.T) {
	a := correctionTestAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not contact broker") })
	for _, input := range [][2]string{{"KRX", "005930_NX"}, {"SOR", "005930_NX"}, {"NXT", "005930_AL"}} {
		if _, err := a.GetQuote(context.Background(), input[0], input[1]); !errors.Is(err, broker.ErrInvalidMarket) {
			t.Errorf("quote err=%v", err)
		}
		if _, err := a.GetOHLCV(context.Background(), input[0], input[1], broker.OHLCVOpts{}); !errors.Is(err, broker.ErrInvalidMarket) {
			t.Errorf("chart err=%v", err)
		}
	}
}

func TestCommonAccountAndChartReadsAllPages(t *testing.T) {
	for _, apiID := range []string{"kt00018", "ka10081"} {
		t.Run(apiID, func(t *testing.T) {
			calls := 0
			a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.Header.Get("api-id") != apiID {
					t.Errorf("api-id=%s", r.Header.Get("api-id"))
				}
				if apiID == "kt00018" && body["qry_tp"] != "1" {
					t.Errorf("qry_tp=%v", body["qry_tp"])
				}
				if calls == 1 {
					if r.Header.Get("cont-yn") != "" || r.Header.Get("next-key") != "" {
						t.Error("unexpected first-page cursor")
					}
					w.Header().Set("cont-yn", "Y")
					w.Header().Set("next-key", "page2")
				} else if r.Header.Get("cont-yn") != "Y" || r.Header.Get("next-key") != "page2" {
					t.Error("missing second-page cursor")
				}
				response := map[string]any{"return_code": 0}
				if apiID == "kt00018" {
					response["acnt_evlt_remn_indv_tot"] = []map[string]string{{"stk_cd": []string{"005930", "000660"}[calls-1], "rmnd_qty": "1"}}
				} else {
					response["stk_dt_pole_chart_qry"] = []map[string]string{{"dt": []string{"20260911", "20260910"}[calls-1], "cur_prc": "70000"}}
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			if apiID == "kt00018" {
				rows, err := a.GetPositions(context.Background(), "test")
				if err != nil || len(rows) != 2 {
					t.Fatalf("rows=%+v err=%v", rows, err)
				}
			} else {
				rows, err := a.GetOHLCV(context.Background(), "KRX", "005930", broker.OHLCVOpts{Interval: "1d"})
				if err != nil || len(rows) != 2 {
					t.Fatalf("rows=%+v err=%v", rows, err)
				}
			}
			if calls != 2 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestCommonPaginationDiscardsPartialResults(t *testing.T) {
	for _, apiID := range []string{"kt00018", "ka10081"} {
		for _, failure := range []string{"http", "business", "repeated", "missing-key"} {
			t.Run(apiID+"/"+failure, func(t *testing.T) {
				calls := 0
				a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					if calls == 2 && failure == "http" {
						http.Error(w, "failure", http.StatusBadGateway)
						return
					}
					if calls == 2 && failure == "business" {
						_, _ = w.Write([]byte(`{"return_code":1,"return_msg":"failed"}`))
						return
					}
					w.Header().Set("cont-yn", "Y")
					if failure != "missing-key" {
						w.Header().Set("next-key", "same")
					}
					_, _ = w.Write([]byte(`{"return_code":0,"acnt_evlt_remn_indv_tot":[{"stk_cd":"005930","rmnd_qty":"1"}],"stk_dt_pole_chart_qry":[{"dt":"20260911","cur_prc":"70000"}]}`))
				})
				if apiID == "kt00018" {
					rows, err := a.GetPositions(context.Background(), "test")
					if err == nil || rows != nil {
						t.Fatalf("partial rows=%v err=%v", rows, err)
					}
				} else {
					rows, err := a.GetOHLCV(context.Background(), "KRX", "005930", broker.OHLCVOpts{})
					if err == nil || rows != nil {
						t.Fatalf("partial rows=%v err=%v", rows, err)
					}
				}
			})
		}
	}
}

func TestCommonPaginationBoundReturnsError(t *testing.T) {
	calls := 0
	a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("cont-yn", "Y")
		w.Header().Set("next-key", strings.Repeat("x", calls))
		_, _ = w.Write([]byte(`{"return_code":0,"acnt_evlt_remn_indv_tot":[{"stk_cd":"005930","rmnd_qty":"1"}]}`))
	})
	rows, err := a.collectEndpointRows(context.Background(), kiwoom.PathAccount, "kt00018", map[string]any{"qry_tp": "1", "dmst_stex_tp": "KRX"}, "acnt_evlt_remn_indv_tot", 2)
	if err == nil || !strings.Contains(err.Error(), "page limit") || rows != nil || calls != 2 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls)
	}
}

func TestRawEndpointPagePreservesCursorAndLegacyBody(t *testing.T) {
	calls := 0
	a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 && (r.Header.Get("cont-yn") != "Y" || r.Header.Get("next-key") != "cursor1") {
			t.Error("request cursor not forwarded")
		}
		w.Header().Set("cont-yn", "Y")
		w.Header().Set("next-key", "cursor2")
		_, _ = w.Write([]byte(`{"return_code":0,"stk_cd":"005930","cur_prc":"70000"}`))
	})
	request := map[string]any{"stk_cd": "005930"}
	page, err := a.CallEndpointPage(context.Background(), "POST", kiwoom.PathStockInfo, "ka10001", request, kiwoomspecs.Continuation{ContYN: "Y", NextKey: "cursor1"})
	if err != nil {
		t.Fatal(err)
	}
	if page.ContYN != "Y" || page.NextKey != "cursor2" {
		t.Fatalf("page=%+v", page)
	}
	legacy, err := a.CallEndpoint(context.Background(), "POST", kiwoom.PathStockInfo, "ka10001", request)
	if err != nil || !reflect.DeepEqual(legacy, page.Data) || calls != 2 {
		t.Fatalf("legacy=%+v page=%+v err=%v calls=%d", legacy, page, err, calls)
	}
}

func TestRawEndpointPageRejectsInvalidContinuation(t *testing.T) {
	a := correctionTestAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not contact broker") })
	for _, continuation := range []kiwoomspecs.Continuation{{ContYN: "Y"}, {NextKey: "key"}, {ContYN: "N", NextKey: "key"}, {ContYN: "bad"}, {ContYN: "Y", NextKey: "x\r\ny"}} {
		_, err := a.CallEndpointPage(context.Background(), "POST", kiwoom.PathStockInfo, "ka10001", map[string]any{"stk_cd": "005930"}, continuation)
		if !errors.Is(err, broker.ErrInvalidOrderRequest) {
			t.Errorf("continuation=%+v err=%v", continuation, err)
		}
	}
}

func TestBalanceUsesWithdrawableAmount(t *testing.T) {
	for _, amount := range []string{"700000", "0", ""} {
		t.Run("withdrawable="+amount, func(t *testing.T) {
			a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"ord_alowa": "800000", "pymn_alow_amt": amount})
			})
			balance, err := a.GetBalance(context.Background(), "test")
			if err != nil {
				t.Fatal(err)
			}
			if balance.BuyingPower != 800000 || balance.WithdrawableCash != parseFloatString(amount) {
				t.Fatalf("balance=%+v", balance)
			}
			if slices.Contains(balance.UnavailableFields, "withdrawable_cash") != (amount == "") {
				t.Fatalf("availability=%v", balance.UnavailableFields)
			}
		})
	}
}

func TestPositionsRejectUndocumentedQueryType(t *testing.T) {
	a := correctionTestAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not contact broker") })
	_, err := a.CallEndpoint(context.Background(), "POST", kiwoom.PathAccount, "kt00018", map[string]any{"qry_tp": "0", "dmst_stex_tp": "KRX"})
	if !errors.Is(err, broker.ErrInvalidOrderRequest) {
		t.Fatalf("err=%v", err)
	}
}

func TestChartStopsWhenRequestedBarsAreCovered(t *testing.T) {
	date := func(day int) time.Time { return time.Date(2026, 9, day, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60)) }
	for _, test := range []struct {
		name  string
		opts  broker.OHLCVOpts
		calls int
		days  []int
	}{
		{"first page has latest limit", broker.OHLCVOpts{Limit: 1}, 1, []int{11}},
		{"second page needed for latest limit", broker.OHLCVOpts{Limit: 3}, 2, []int{11, 10, 9}},
		{"first page reaches from", broker.OHLCVOpts{From: date(10)}, 1, []int{11, 10}},
		{"calendar date survives negative offset", broker.OHLCVOpts{From: time.Date(2026, 9, 10, 0, 0, 0, 0, time.FixedZone("West", -7*60*60))}, 1, []int{11, 10}},
		{"second page reaches range", broker.OHLCVOpts{From: date(9), To: date(10)}, 2, []int{10, 9}},
		{"limit counts only in-range bars", broker.OHLCVOpts{To: date(10), Limit: 2}, 2, []int{10, 9}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > test.calls {
					t.Error("requested an unnecessary older page")
					http.Error(w, "unneeded old page", 500)
					return
				}
				if calls > 1 && (r.Header.Get("cont-yn") != "Y" || r.Header.Get("next-key") != "page2") {
					t.Error("missing page2 request cursor")
				}
				w.Header().Set("cont-yn", "Y")
				w.Header().Set("next-key", []string{"page2", "page3"}[calls-1])
				rows := []map[string]string{{"dt": date(13 - 2*calls).Format("20060102"), "cur_prc": "70000"}, {"dt": date(12 - 2*calls).Format("20060102"), "cur_prc": "69000"}}
				_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "stk_dt_pole_chart_qry": rows})
			})
			rows, err := a.GetOHLCV(context.Background(), "KRX", "005930", test.opts)
			if err != nil {
				t.Fatal(err)
			}
			days := make([]int, 0, len(rows))
			for _, row := range rows {
				days = append(days, row.Timestamp.Day())
			}
			if calls != test.calls || !slices.Equal(days, test.days) {
				t.Fatalf("calls=%d days=%v, want calls=%d days=%v", calls, days, test.calls, test.days)
			}
		})
	}
}

func TestChartEarlyStopStillRejectsMalformedPages(t *testing.T) {
	for _, scenario := range []string{"malformed date", "ascending dates", "missing cursor", "repeated cursor"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("cont-yn", "Y")
				if scenario != "missing cursor" {
					w.Header().Set("next-key", "same")
				}
				dates := []string{"20260911", "20260910"}
				switch scenario {
				case "malformed date":
					dates[1] = "invalid"
				case "ascending dates":
					dates = []string{"20260910", "20260911"}
				case "repeated cursor":
					if calls == 2 {
						dates = []string{"20260909", "20260908"}
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "stk_dt_pole_chart_qry": []map[string]string{{"dt": dates[0], "cur_prc": "70000"}, {"dt": dates[1], "cur_prc": "69000"}}})
			})
			limit := 1
			if scenario == "repeated cursor" {
				limit = 3
			}
			rows, err := a.GetOHLCV(context.Background(), "KRX", "005930", broker.OHLCVOpts{Limit: limit})
			if err == nil || rows != nil {
				t.Fatalf("malformed partial rows escaped: rows=%v err=%v", rows, err)
			}
		})
	}
}

func TestPositionsRejectMalformedRowsInsteadOfDroppingHoldings(t *testing.T) {
	for _, badRow := range []any{nil, map[string]string{"stk_cd": "005930"}, map[string]string{"stk_cd": "005930", "rmnd_qty": "unknown"}} {
		a := correctionTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"return_code": 0, "acnt_evlt_remn_indv_tot": []any{map[string]string{"stk_cd": "000660", "rmnd_qty": "1"}, badRow}})
		})
		positions, err := a.GetPositions(context.Background(), "test")
		if err == nil || positions != nil {
			t.Fatalf("malformed holding silently dropped: rows=%v err=%v", positions, err)
		}
	}
}
