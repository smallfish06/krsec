package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallfish06/krsec/internal/ls"
	"github.com/smallfish06/krsec/pkg/broker"
)

func lifecycleAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == ls.PathOAuthToken {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "local-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	a := NewAdapterWithOptions(false, "local-account", &testTokenManager{}, "", nil, t.TempDir())
	a.now = func() time.Time { return time.Date(2026, 9, 11, 14, 30, 0, 0, lsOrderLocation) }
	a.Client().SetBaseURL(srv.URL)
	if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: t.Name(), AppSecret: "local-secret"}); err != nil {
		t.Fatal(err)
	}
	return a
}

func trackLifecycleOrder(a *Adapter, id string) {
	a.storeOrderContext(id, orderContext{OrderID: id, AccountID: a.accountID, CredentialID: a.credentialID, Symbol: "005930", Market: "KRX", Side: broker.OrderSideBuy, OrderType: broker.OrderTypeLimit, Quantity: 10, Price: 70000, PlacedAt: a.orderNow()})
}

func lifecycleOrderRow(id string) map[string]any {
	return map[string]any{"ordno": id, "expcode": "005930", "medosu": "매수", "qty": 10, "price": 70000, "cheqty": 7, "cheprice": 69900, "ordrem": 3, "cfmqty": 0, "status": "접수", "ordtime": "14:00:00", "hogagb": "00"}
}

func writeLifecycleOrders(w http.ResponseWriter, rows []map[string]any, cursor string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "t0425OutBlock": map[string]any{"cts_ordno": cursor}, "t0425OutBlock1": rows})
}

func TestLSPlaceThenCancelUsesLiveUnfilledQuantity(t *testing.T) {
	t.Parallel()
	var cancelled bool
	a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("tr_cd") {
		case "CSPAT00601":
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAT00601OutBlock2": map[string]any{"OrdNo": 123}})
		case "t0425":
			writeLifecycleOrders(w, []map[string]any{lifecycleOrderRow("0000000123")}, "")
		case "CSPAT00801":
			var body map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			block := body["CSPAT00801InBlock1"]
			if block["OrgOrdNo"] != float64(123) || block["IsuNo"] != "005930" || block["OrdQty"] != float64(3) {
				t.Errorf("unsafe cancellation payload: %#v", block)
			}
			cancelled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAT00801OutBlock2": map[string]any{"OrdNo": 124}})
		default:
			t.Errorf("unexpected TR: %s", r.Header.Get("tr_cd"))
		}
	})
	order, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Symbol: "005930", Market: "KRX", Side: broker.OrderSideBuy, Type: broker.OrderTypeLimit, Quantity: 10, Price: 70000})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.CancelOrder(context.Background(), order.OrderID); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("cancel mutation never reached provider")
	}
}

func TestLSLargeNumericOrderIDsAndQuantitiesRemainExact(t *testing.T) {
	t.Parallel()
	for _, id := range []int64{12345678, 1234567890, 9999999999} {
		t.Run(strconv.FormatInt(id, 10), func(t *testing.T) {
			t.Parallel()
			const quantity int64 = 1234567
			var cancelled bool
			a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Header.Get("tr_cd") {
				case "CSPAT00601":
					_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAT00601OutBlock2": map[string]any{"OrdNo": id}})
				case "t0425":
					row := lifecycleOrderRow("")
					row["ordno"] = id
					row["qty"] = quantity
					row["cheqty"] = quantity - 3
					writeLifecycleOrders(w, []map[string]any{row}, "")
				case "CSPAT00801":
					var payload map[string]map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&payload); err != nil {
						t.Error(err)
					}
					block := payload["CSPAT00801InBlock1"]
					if anyString(block["OrgOrdNo"]) != strconv.FormatInt(id, 10) || anyString(block["OrdQty"]) != "3" {
						t.Errorf("numeric identity changed: %#v", block)
					}
					cancelled = true
					_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAT00801OutBlock2": map[string]any{"OrdNo": 9999999998}})
				default:
					t.Errorf("unexpected TR %s", r.Header.Get("tr_cd"))
				}
			})
			placed, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Symbol: "005930", Market: "KRX", Side: broker.OrderSideBuy, Type: broker.OrderTypeLimit, Quantity: quantity, Price: 70000})
			if err != nil {
				t.Fatal(err)
			}
			if placed.OrderID != strconv.FormatInt(id, 10) {
				t.Fatalf("accepted order ID changed: %+v", placed)
			}
			order, err := a.GetOrder(context.Background(), placed.OrderID)
			if err != nil {
				t.Fatal(err)
			}
			if order.FilledQuantity != quantity-3 || order.RemainingQty != 3 {
				t.Fatalf("numeric quantities changed: %+v", order)
			}
			if err = a.CancelOrder(context.Background(), placed.OrderID); err != nil {
				t.Fatal(err)
			}
			if !cancelled {
				t.Fatal("order did not reach cancellation")
			}
		})
	}
}

func TestLSPlaceOrderRejectsUnroutableQuoteMarketAliases(t *testing.T) {
	t.Parallel()
	for _, market := range []string{"U", "UNIFIED", "ALL", "SOR", "NASDAQ"} {
		t.Run(market, func(t *testing.T) {
			t.Parallel()
			a := lifecycleAdapter(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unsupported order market reached provider") })
			_, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Symbol: "005930", Market: market, Side: broker.OrderSideBuy, Type: broker.OrderTypeLimit, Quantity: 1, Price: 70000})
			if !errors.Is(err, broker.ErrInvalidMarket) {
				t.Fatalf("market %s was routed to KRX: %v", market, err)
			}
		})
	}
}

func TestLSModifyTracksNewOrderAndKeepsOriginalContext(t *testing.T) {
	t.Parallel()
	a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("tr_cd") {
		case "t0425":
			writeLifecycleOrders(w, []map[string]any{lifecycleOrderRow("123")}, "")
		case "CSPAT00701":
			var body map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			b := body["CSPAT00701InBlock1"]
			if b["OrgOrdNo"] != float64(123) || b["OrdQty"] != float64(3) || b["OrdPrc"] != float64(70100) || b["OrdprcPtnCode"] != "00" {
				t.Errorf("wrong modify request: %#v", b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAT00701OutBlock2": map[string]any{"OrdNo": 125}})
		default:
			t.Errorf("unexpected TR: %s", r.Header.Get("tr_cd"))
		}
	})
	trackLifecycleOrder(a, "123")
	result, err := a.ModifyOrder(context.Background(), "123", broker.ModifyOrderRequest{Price: 70100})
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderID != "125" || result.RemainingQty != 3 || result.Status != broker.OrderStatusPending {
		t.Fatalf("result=%+v", result)
	}
	if _, err = a.trackedOrder("125"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.trackedOrder("123"); err != nil {
		t.Fatalf("original status context lost: %v", err)
	}
}

func TestLSOrderLookupTraversesBothContinuationForms(t *testing.T) {
	t.Parallel()
	for _, headers := range []bool{false, true} {
		t.Run(map[bool]string{false: "body cursor", true: "body and headers"}[headers], func(t *testing.T) {
			t.Parallel()
			pages := 0
			a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				pages++
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if pages == 1 {
					if headers {
						w.Header().Set("tr_cont", "Y")
						w.Header().Set("tr_cont_key", "page-2")
					}
					writeLifecycleOrders(w, []map[string]any{lifecycleOrderRow("999")}, "900")
					return
				}
				if body["t0425InBlock"]["cts_ordno"] != "900" {
					t.Errorf("body cursor lost: %#v", body)
				}
				if headers && (r.Header.Get("tr_cont") != "Y" || r.Header.Get("tr_cont_key") != "page-2") {
					t.Error("header continuation lost")
				}
				writeLifecycleOrders(w, []map[string]any{lifecycleOrderRow("123")}, "")
			})
			trackLifecycleOrder(a, "123")
			order, err := a.GetOrder(context.Background(), "000123")
			if err != nil {
				t.Fatal(err)
			}
			if pages != 2 || order.FilledQuantity != 7 || order.RemainingQty != 3 || order.AvgFilledPrice != 0 {
				t.Fatalf("pages=%d result=%+v", pages, order)
			}
		})
	}
}

func TestLSCancellationRejectsUnknownExpiredAndDifferentIdentity(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"unknown", "prior day", "different account", "different app key"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			a := lifecycleAdapter(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unsafe order lookup reached provider") })
			if scope != "unknown" {
				trackLifecycleOrder(a, "123")
			}
			switch scope {
			case "prior day":
				a.orders["123"] = func() orderContext { m := a.orders["123"]; m.PlacedAt = m.PlacedAt.AddDate(0, 0, -1); return m }()
			case "different account":
				a.accountID = "another-account"
			case "different app key":
				a.credentialID = credentialFingerprint("another-key")
			}
			if err := a.CancelOrder(context.Background(), "123"); !errors.Is(err, broker.ErrNotSupported) {
				t.Fatalf("unsafe context accepted: %v", err)
			}
		})
	}
}

func TestLSCancellationRejectsAmbiguousIncompleteAndChangedOrders(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"symbol", "side", "duplicate", "page failure", "repeated cursor", "missing remaining"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			calls := 0
			a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("tr_cd") != "t0425" {
					t.Error("unsafe cancellation mutation")
					return
				}
				calls++
				row := lifecycleOrderRow("123")
				switch failure {
				case "symbol":
					row["expcode"] = "000660"
				case "side":
					row["medosu"] = "매도"
				case "missing remaining":
					delete(row, "ordrem")
				case "duplicate":
					other := lifecycleOrderRow("123")
					other["ordrem"] = 2
					writeLifecycleOrders(w, []map[string]any{row, other}, "")
					return
				case "page failure":
					if calls > 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writeLifecycleOrders(w, []map[string]any{row}, "900")
					return
				case "repeated cursor":
					writeLifecycleOrders(w, []map[string]any{row}, "900")
					return
				}
				writeLifecycleOrders(w, []map[string]any{row}, "")
			})
			trackLifecycleOrder(a, "123")
			if err := a.CancelOrder(context.Background(), "123"); err == nil {
				t.Fatal("unsafe order accepted")
			}
		})
	}
}

func TestLSFillsUseRecordedDateAndExecutionTimesAcrossPages(t *testing.T) {
	t.Parallel()
	pages := 0
	a := lifecycleAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.Header.Get("tr_cd") != "CSPAQ13700" {
			t.Errorf("wrong TR %s", r.Header.Get("tr_cd"))
		}
		var body map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		b := body["CSPAQ13700InBlock1"]
		if b["OrdDt"] != "20260911" || b["ExecYn"] != "1" || b["IsuNo"] != "A005930" || b["BkseqTpCode"] != "1" || b["SrtOrdNo2"] != float64(0) {
			t.Errorf("wrong history scope %#v", b)
		}
		row := map[string]any{"OrdNo": 123, "OrdDt": "20260911", "IsuNo": "A005930", "BnsTpCode": "2", "ExecQty": 2, "ExecPrc": 70000, "ExecTrxTime": "140101123", "OrdTrxPtnNm": "체결"}
		if pages == 1 {
			w.Header().Set("tr_cont", "Y")
			w.Header().Set("tr_cont_key", "fills-next")
		} else {
			if r.Header.Get("tr_cont_key") != "fills-next" {
				t.Error("fill continuation missing")
			}
			row["ExecQty"] = 3
			row["ExecTrxTime"] = "140203456"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "CSPAQ13700OutBlock3": []map[string]any{row}})
	})
	trackLifecycleOrder(a, "123")
	fills, err := a.GetOrderFills(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 2 || fills[0].Quantity != 2 || fills[1].Quantity != 3 || fills[0].Amount != 140000 || fills[0].FilledAt.Format("20060102150405.000") != "20260911140101.123" {
		t.Fatalf("fills=%+v", fills)
	}
}

func TestLSOrderContextSurvivesRestartWithoutCrossAccountAccess(t *testing.T) {
	t.Parallel()
	a := lifecycleAdapter(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unexpected request") })
	trackLifecycleOrder(a, "123")
	b := NewAdapterWithOptions(false, a.accountID, &testTokenManager{}, "", nil, a.orderDir)
	b.now = a.now
	b.credentialID = a.credentialID
	if _, err := b.trackedOrder("123"); err != nil {
		t.Fatal(err)
	}
	c := NewAdapterWithOptions(false, "different-account", &testTokenManager{}, "", nil, a.orderDir)
	c.now = a.now
	c.credentialID = a.credentialID
	if _, err := c.trackedOrder("123"); !errors.Is(err, broker.ErrNotSupported) {
		t.Fatalf("cross-account context leaked: %v", err)
	}
}

func TestLSFailedReauthenticationInvalidatesTrackedOrderIdentity(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider rejects credentials", true: "caller cancels token issuance"}[canceled], func(t *testing.T) {
			t.Parallel()
			var accountRequests atomic.Int32
			secondAuthStarted := make(chan struct{})
			releaseSecondAuth := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != ls.PathOAuthToken {
					accountRequests.Add(1)
					http.Error(w, "must not query any account", http.StatusForbidden)
					return
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("appkey") == t.Name()+"-B" {
					close(secondAuthStarted)
					if canceled {
						<-releaseSecondAuth
					} else {
						w.WriteHeader(http.StatusUnauthorized)
						_ = json.NewEncoder(w).Encode(map[string]any{"error_code": "invalid_client", "error_description": "invalid credentials"})
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "local-token", "token_type": "Bearer", "expires_in": 3600})
			}))
			defer srv.Close()
			defer close(releaseSecondAuth)
			tm := &testTokenManager{}
			a := NewAdapterWithOptions(false, "local-account", tm, "", nil, t.TempDir())
			a.Client().SetBaseURL(srv.URL)
			if _, err := a.Authenticate(context.Background(), broker.Credentials{AppKey: t.Name() + "-A", AppSecret: "local-secret"}); err != nil {
				t.Fatal(err)
			}
			trackLifecycleOrder(a, "123")
			// This test manager has a single slot; clear it to force issuance for B.
			if err := tm.DeleteToken(t.Name() + "-A"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			authResult := make(chan error, 1)
			go func() {
				_, err := a.Authenticate(ctx, broker.Credentials{AppKey: t.Name() + "-B", AppSecret: "other-secret"})
				authResult <- err
			}()
			<-secondAuthStarted
			if canceled {
				cancel()
			}
			if err := <-authResult; err == nil {
				t.Fatal("reauthentication unexpectedly succeeded")
			}
			if a.credentialID != "" {
				t.Fatal("old account fingerprint survived failed reauthentication")
			}
			_, lookupErr := a.GetOrder(context.Background(), "123")
			cancelErr := a.CancelOrder(context.Background(), "123")
			_, modifyErr := a.ModifyOrder(context.Background(), "123", broker.ModifyOrderRequest{Price: 70100})
			_, fillsErr := a.GetOrderFills(context.Background(), "123")
			for _, err := range []error{lookupErr, cancelErr, modifyErr, fillsErr} {
				if !errors.Is(err, broker.ErrUnauthorized) {
					t.Fatalf("tracked order operation must fail closed: %v", err)
				}
			}
			if accountRequests.Load() != 0 {
				t.Fatalf("%d account requests escaped identity guard", accountRequests.Load())
			}
		})
	}
}

func TestLSBalanceUsesEstimatedNetAssetsAndMarksCashUnavailable(t *testing.T) {
	t.Parallel()
	a := lifecycleAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "t0424OutBlock": map[string]any{"sunamt": 130000, "sunamt1": 30000, "tappamt": 100000, "tdtsunik": 10000, "mamt": 90000, "cts_expcode": ""}, "t0424OutBlock1": []map[string]any{}})
	})
	balance, err := a.GetBalance(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if balance.TotalAssets != 130000 || balance.PositionValue != 100000 || balance.Cash != 0 || balance.BuyingPower != 0 || !slices.Contains(balance.UnavailableFields, "cash") || !slices.Contains(balance.UnavailableFields, "buying_power") {
		t.Fatalf("wrong balance semantics: %+v", balance)
	}
}
