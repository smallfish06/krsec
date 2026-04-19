package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestAdapter_IntegratedCoreFlows(t *testing.T) {
	orderID := "0001001"
	fillOrderID := "0002002"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "kiwoom-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/stkinfo":
			switch r.Header.Get("api-id") {
			case "ka10001":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"stk_cd":      "005930",
					"stk_nm":      "삼성전자",
					"cur_prc":     "70000",
					"open_pric":   "69500",
					"high_pric":   "70500",
					"low_pric":    "69000",
					"pred_pre":    "500",
					"flu_rt":      "0.72",
					"trde_qty":    "1234567",
					"base_pric":   "69500",
					"upl_pric":    "91000",
					"lst_pric":    "48000",
					"return_code": 0,
					"return_msg":  "ok",
				})
			case "ka10100":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"code":             "005930",
					"name":             "삼성전자",
					"listCount":        "5969782550",
					"regDay":           "19750611",
					"state":            "정상",
					"marketCode":       "0",
					"marketName":       "거래소",
					"upName":           "전기전자",
					"return_code":      0,
					"return_msg":       "ok",
					"companyClassName": "",
				})
			default:
				http.NotFound(w, r)
			}
		case "/api/dostk/chart":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"stk_cd": "005930",
				"stk_dt_pole_chart_qry": []map[string]interface{}{
					{"dt": "20260227", "open_pric": "70000", "high_pric": "71000", "low_pric": "69500", "cur_prc": "70500", "trde_qty": "1000"},
					{"dt": "20260226", "open_pric": "69000", "high_pric": "70000", "low_pric": "68500", "cur_prc": "69800", "trde_qty": "2000"},
				},
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/acnt":
			switch r.Header.Get("api-id") {
			case "kt00005":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"entr":               "1000000",
					"entr_d1":            "950000",
					"entr_d2":            "900000",
					"ord_alowa":          "800000",
					"wthd_alowa":         "700000",
					"uncl_stk_amt":       "15000",
					"stk_buy_tot_amt":    "3000000",
					"evlt_amt_tot":       "3400000",
					"tot_pl_tot":         "400000",
					"tot_pl_rt":          "13.33",
					"prsm_dpst_aset_amt": "4400000",
					"crd_loan_tot":       "0",
					"return_code":        0,
					"return_msg":         "ok",
				})
			case "kt00018":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"tot_pur_amt":  "3000000",
					"tot_evlt_amt": "3400000",
					"tot_evlt_pl":  "400000",
					"tot_prft_rt":  "13.33",
					"acnt_evlt_remn_indv_tot": []map[string]interface{}{
						{
							"stk_cd":        "A005930",
							"stk_nm":        "삼성전자",
							"rmnd_qty":      "10",
							"trde_able_qty": "8",
							"pur_pric":      "68000",
							"cur_prc":       "70000",
							"pur_amt":       "680000",
							"evlt_amt":      "700000",
							"evltv_prft":    "20000",
							"prft_rt":       "2.94",
							"poss_rt":       "20.5",
							"tdy_buyq":      "1",
							"tdy_sellq":     "0",
							"crd_loan_dt":   "",
						},
					},
					"return_code": 0,
					"return_msg":  "ok",
				})
			case "ka10075":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"oso": []map[string]interface{}{
						{"ord_no": orderID, "stk_cd": "005930", "ord_stt": "접수", "ord_qty": "5", "oso_qty": "5", "ord_pric": "70000", "io_tp_nm": "+매수", "stex_tp": "1", "stex_tp_txt": "KRX"},
					},
					"return_code": 0,
					"return_msg":  "ok",
				})
			case "ka10076":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"cntr": []map[string]interface{}{
						{"ord_no": fillOrderID, "stk_cd": "005930", "io_tp_nm": "+매수", "cntr_pric": "70100", "cntr_qty": "2", "ord_tm": "101010", "ord_stt": "체결", "stex_tp": "1", "stex_tp_txt": "KRX"},
						{"ord_no": fillOrderID, "stk_cd": "005930", "io_tp_nm": "+매수", "cntr_pric": "70200", "cntr_qty": "1", "ord_tm": "101110", "ord_stt": "체결", "stex_tp": "1", "stex_tp_txt": "KRX"},
					},
					"return_code": 0,
					"return_msg":  "ok",
				})
			default:
				http.NotFound(w, r)
			}
		case "/api/dostk/ordr":
			switch r.Header.Get("api-id") {
			case "kt10000":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"ord_no": orderID, "return_code": 0, "return_msg": "ok"})
			case "kt10003":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"ord_no": "0001002", "return_code": 0, "return_msg": "ok"})
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "1234567890", &testTokenManager{}, t.TempDir(), nil)
	a.client.SetBaseURL(ts.URL)

	ctx := context.Background()
	if _, err := a.Authenticate(ctx, broker.Credentials{AppKey: "k", AppSecret: "s"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	quote, err := a.GetQuote(ctx, "KRX", "005930")
	if err != nil {
		t.Fatalf("GetQuote error: %v", err)
	}
	if quote.Price != 70000 || quote.Change != 500 || quote.ChangeRate != 0.72 {
		t.Fatalf("unexpected quote: %+v", quote)
	}

	ohlcv, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1d", Limit: 1})
	if err != nil {
		t.Fatalf("GetOHLCV error: %v", err)
	}
	if len(ohlcv) != 1 || ohlcv[0].Close != 70500 {
		t.Fatalf("unexpected ohlcv: %+v", ohlcv)
	}

	bal, err := a.GetBalance(ctx, "1234567890")
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if bal.Cash != 1000000 || bal.BuyingPower != 800000 || bal.PositionValue != 3400000 {
		t.Fatalf("unexpected balance: %+v", bal)
	}

	pos, err := a.GetPositions(ctx, "1234567890")
	if err != nil {
		t.Fatalf("GetPositions error: %v", err)
	}
	if len(pos) != 1 || pos[0].Symbol != "005930" || pos[0].Quantity != 10 {
		t.Fatalf("unexpected positions: %+v", pos)
	}

	placed, err := a.PlaceOrder(ctx, broker.OrderRequest{
		AccountID: "1234567890",
		Symbol:    "005930",
		Market:    "KRX",
		Side:      broker.OrderSideBuy,
		Type:      broker.OrderTypeLimit,
		Quantity:  5,
		Price:     70000,
	})
	if err != nil {
		t.Fatalf("PlaceOrder error: %v", err)
	}
	if placed.OrderID != orderID || placed.Status != broker.OrderStatusPending {
		t.Fatalf("unexpected place result: %+v", placed)
	}

	ord, err := a.GetOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetOrder error: %v", err)
	}
	if ord.OrderID != orderID || ord.Status != broker.OrderStatusPending || ord.RemainingQty != 5 {
		t.Fatalf("unexpected order: %+v", ord)
	}

	fills, err := a.GetOrderFills(ctx, fillOrderID)
	if err != nil {
		t.Fatalf("GetOrderFills error: %v", err)
	}
	if len(fills) != 2 {
		t.Fatalf("fills length = %d, want 2", len(fills))
	}
	if fills[0].Quantity != 2 || fills[1].Quantity != 1 {
		t.Fatalf("unexpected fills: %+v", fills)
	}

	if err := a.CancelOrder(ctx, orderID); err != nil {
		t.Fatalf("CancelOrder error: %v", err)
	}

	inst, err := a.GetInstrument(ctx, "KRX", "005930")
	if err != nil {
		t.Fatalf("GetInstrument error: %v", err)
	}
	if inst.Symbol != "005930" || inst.Name != "삼성전자" || inst.ListedShares == 0 {
		t.Fatalf("unexpected instrument: %+v", inst)
	}
}

func TestAdapter_GetOrderFills_OrderNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"expires_dt":  "20991231235959",
				"token_type":  "bearer",
				"token":       "kiwoom-token",
				"return_code": 0,
				"return_msg":  "ok",
			})
		case "/api/dostk/acnt":
			if r.Header.Get("api-id") == "ka10076" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"cntr":        []map[string]interface{}{},
					"return_code": 0,
					"return_msg":  "ok",
				})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	a := NewAdapterWithOptions(false, "1234567890", &testTokenManager{}, t.TempDir(), nil)
	a.client.SetBaseURL(ts.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: "k", AppSecret: "s"}); err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}

	_, err := a.GetOrderFills(context.Background(), "NOT-FOUND")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, broker.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}
