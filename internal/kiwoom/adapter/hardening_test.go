package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

// kiwoomFake answers each api-id with the payload registered for it and
// records the request bodies it received.
type kiwoomFake struct {
	t         *testing.T
	responses map[string]func(body map[string]any) (status int, payload map[string]any)
	calls     map[string]*atomic.Int32
	lastBody  map[string]map[string]any
}

func newKiwoomFake(t *testing.T) *kiwoomFake {
	return &kiwoomFake{
		t:         t,
		responses: map[string]func(map[string]any) (int, map[string]any){},
		calls:     map[string]*atomic.Int32{},
		lastBody:  map[string]map[string]any{},
	}
}

func (f *kiwoomFake) on(apiID string, payload map[string]any) {
	f.onFunc(apiID, func(map[string]any) (int, map[string]any) { return http.StatusOK, payload })
}

func (f *kiwoomFake) onFunc(apiID string, fn func(map[string]any) (int, map[string]any)) {
	f.responses[apiID] = fn
	f.calls[apiID] = &atomic.Int32{}
}

func (f *kiwoomFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("api-id")
	respond, ok := f.responses[apiID]
	if !ok {
		f.t.Errorf("unexpected Kiwoom call %s", apiID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.lastBody[apiID] = body
	f.calls[apiID].Add(1)
	status, payload := respond(body)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// kt10000 is the buy TR and kt10001 the sell TR. An unrecognized side used to
// fall through to the buy TR.
func TestPlaceOrderRejectsUnrecognizedSide(t *testing.T) {
	for _, side := range []broker.OrderSide{"SELL", "Sell", "", "short"} {
		fake := newKiwoomFake(t)
		a := correctionTestAdapter(t, fake.ServeHTTP)
		_, err := a.PlaceOrder(context.Background(), broker.OrderRequest{
			Market: "KRX", Symbol: "005930", Side: side, Type: broker.OrderTypeLimit, Quantity: 1, Price: 70000,
		})
		if !errors.Is(err, broker.ErrInvalidOrderRequest) {
			t.Fatalf("side %q: err = %v, want ErrInvalidOrderRequest", side, err)
		}
	}
}

func TestPlaceOrderRejectsOptionsKiwoomCannotExpress(t *testing.T) {
	base := broker.OrderRequest{Market: "KRX", Symbol: "005930", Side: broker.OrderSideBuy, Type: broker.OrderTypeLimit, Quantity: 1, Price: 70000}
	for name, mutate := range map[string]func(*broker.OrderRequest){
		"ioc":        func(r *broker.OrderRequest) { r.TimeInForce = "IOC" },
		"fractional": func(r *broker.OrderRequest) { r.QuantityDecimal = "0.5" },
		"by amount":  func(r *broker.OrderRequest) { r.OrderAmount = 100000 },
	} {
		t.Run(name, func(t *testing.T) {
			a := correctionTestAdapter(t, newKiwoomFake(t).ServeHTTP)
			req := base
			mutate(&req)
			if _, err := a.PlaceOrder(context.Background(), req); !errors.Is(err, broker.ErrInvalidOrderRequest) {
				t.Fatalf("err = %v, want ErrInvalidOrderRequest", err)
			}
		})
	}
}

func TestGetOrderDoesNotGuessWhenLookupsFail(t *testing.T) {
	fake := newKiwoomFake(t)
	fake.onFunc("ka10075", func(map[string]any) (int, map[string]any) {
		return http.StatusInternalServerError, map[string]any{"return_code": 9, "return_msg": "system busy"}
	})
	a := correctionTestAdapter(t, fake.ServeHTTP)
	a.storeOrderContext("1001", orderContext{Symbol: "005930", Exchange: "KRX", Quantity: 100, RemainingQty: 100, Status: broker.OrderStatusPending})

	if got, err := a.GetOrder(context.Background(), "1001"); err == nil {
		t.Fatalf("open-order lookup failed but GetOrder returned %+v", got)
	}
}

// An order that left the open list after filling 40 of 100 was cancelled for
// the rest; it used to be reported as filled with nothing remaining.
func TestGetOrderPartialFillThenClosedIsNotFilled(t *testing.T) {
	fake := newKiwoomFake(t)
	fake.on("ka10075", map[string]any{"return_code": 0, "oso": []any{}})
	fake.on("ka10076", map[string]any{"return_code": 0, "cntr": []any{
		map[string]any{"ord_no": "1001", "stk_cd": "005930", "cntr_qty": "40", "cntr_pric": "70000", "ord_tm": "093000"},
	}})
	a := correctionTestAdapter(t, fake.ServeHTTP)
	a.storeOrderContext("1001", orderContext{Symbol: "005930", Exchange: "KRX", Quantity: 100, RemainingQty: 100, Status: broker.OrderStatusPending})

	got, err := a.GetOrder(context.Background(), "1001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != broker.OrderStatusCancelled || got.FilledQuantity != 40 || got.AvgFilledPrice != 70000 {
		t.Fatalf("unexpected order state %+v", got)
	}
}

// The cached remaining quantity is stale after a partial fill; cancelling the
// original 100 would be rejected and leave 60 shares working.
func TestCancelOrderUsesLiveOpenQuantity(t *testing.T) {
	fake := newKiwoomFake(t)
	fake.on("ka10075", map[string]any{"return_code": 0, "oso": []any{
		map[string]any{"ord_no": "1001", "stk_cd": "005930", "ord_qty": "100", "oso_qty": "60"},
	}})
	fake.on("kt10003", map[string]any{"return_code": 0, "ord_no": "1002"})
	a := correctionTestAdapter(t, fake.ServeHTTP)
	a.storeOrderContext("1001", orderContext{Symbol: "005930", Exchange: "KRX", Quantity: 100, RemainingQty: 100, Status: broker.OrderStatusPending})

	if err := a.CancelOrder(context.Background(), "1001"); err != nil {
		t.Fatal(err)
	}
	if got := fake.lastBody["kt10003"]["cncl_qty"]; got != "60" {
		t.Fatalf("cncl_qty = %v, want the 60 shares still open", got)
	}
}

// After a modify, the original order number must keep addressing the original
// order rather than a copy of the new order's quantity.
func TestModifyOrderKeepsOriginalOrderContext(t *testing.T) {
	fake := newKiwoomFake(t)
	fake.on("kt10002", map[string]any{"return_code": 0, "ord_no": "2002"})
	a := correctionTestAdapter(t, fake.ServeHTTP)
	a.storeOrderContext("1001", orderContext{Symbol: "005930", Exchange: "KRX", Quantity: 100, RemainingQty: 100, Price: 70000, Status: broker.OrderStatusPending})

	result, err := a.ModifyOrder(context.Background(), "1001", broker.ModifyOrderRequest{Quantity: 30, Price: 71000})
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderID != "2002" {
		t.Fatalf("modified order id = %q, want 2002", result.OrderID)
	}
	original, _ := a.getOrderContext("1001")
	if original.OrderID != "1001" || original.Quantity != 100 {
		t.Fatalf("original order context was overwritten: %+v", original)
	}
	modified, _ := a.getOrderContext("2002")
	if modified.Quantity != 30 || modified.Price != 71000 {
		t.Fatalf("modified order context = %+v", modified)
	}
}

func TestRejectedTokenIsDroppedSoTheNextCallReauthenticates(t *testing.T) {
	fake := newKiwoomFake(t)
	fake.on("ka10075", map[string]any{"return_code": 3, "return_msg": "인증에 실패했습니다[8005:Token이 유효하지 않습니다]"})
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	tm := &testTokenManager{token: "stale-token", expiresAt: time.Now().Add(time.Hour), hasToken: true}
	a := NewAdapterWithOptions(false, "test", tm, t.TempDir(), nil)
	a.client.SetBaseURL(server.URL)
	a.client.SetCredentials("test-key", "test-secret")

	if _, err := a.fetchUnsettled(context.Background(), ""); !errors.Is(err, broker.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if tm.hasToken {
		t.Fatal("rejected token is still cached; every call would fail until its recorded expiry")
	}
}
