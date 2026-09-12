package adapter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/pkg/broker"
)

func newStubAdapter(t *testing.T) *Adapter {
	t.Helper()
	a := &Adapter{accountID: "12345678", accountPrdtCD: "01", orderDir: t.TempDir(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), orders: make(map[string]orderContext)}
	a.dispatcher = &endpointDispatcher{adapter: a, routes: make(map[string]endpointRoute)}
	return a
}

func TestPositionsRequireEverySourceToSucceed(t *testing.T) {
	t.Parallel()
	upstream := errors.New("upstream authentication failed")
	for _, failed := range []string{"stock", "bond", "neither"} {
		t.Run(failed, func(t *testing.T) {
			a := newStubAdapter(t)
			a.dispatcher.routes[kis.PathDomesticStockTradingInquireBalance] = newEndpointRoute([]string{http.MethodGet}, func(context.Context, string, string, map[string]string) (any, error) {
				if failed == "stock" {
					return nil, upstream
				}
				return map[string]any{"output1": []any{map[string]string{"pdno": "005930", "hldg_qty": "2"}}}, nil
			})
			a.dispatcher.routes[kis.PathDomesticBondInquireBalance] = newEndpointRoute([]string{http.MethodGet}, func(context.Context, string, string, map[string]string) (any, error) {
				if failed == "bond" {
					return nil, upstream
				}
				return map[string]any{"output": []any{map[string]string{"pdno": "bond", "cblc_qty": "3", "buy_amt": "3000"}}}, nil
			})
			rows, err := a.GetPositions(context.Background(), "12345678-01")
			if failed != "neither" {
				if !errors.Is(err, upstream) || rows != nil {
					t.Fatalf("failed source must not become complete holdings: rows=%v err=%v", rows, err)
				}
			} else if err != nil || len(rows) != 2 {
				t.Fatalf("complete sources: rows=%v err=%v", rows, err)
			}
		})
	}
}

func TestSandboxPositionsExcludeUnsupportedBondSource(t *testing.T) {
	t.Parallel()
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "held", true: "empty"}[empty], func(t *testing.T) {
			a := newStubAdapter(t)
			a.sandbox = true
			a.dispatcher.routes[kis.PathDomesticStockTradingInquireBalance] = newEndpointRoute([]string{http.MethodGet}, func(context.Context, string, string, map[string]string) (any, error) {
				rows := []any{}
				if !empty {
					rows = append(rows, map[string]string{"pdno": "005930", "hldg_qty": "2"})
				}
				return map[string]any{"output1": rows}, nil
			})
			a.dispatcher.routes[kis.PathDomesticBondInquireBalance] = newEndpointRoute([]string{http.MethodGet}, func(context.Context, string, string, map[string]string) (any, error) {
				t.Fatal("bond balance is documented as unsupported in virtual trading")
				return nil, nil
			})
			rows, err := a.GetPositions(context.Background(), "12345678-01")
			want := 1
			if empty {
				want = 0
			}
			if err != nil || rows == nil || len(rows) != want {
				t.Fatalf("supported sandbox holdings: rows=%v err=%v", rows, err)
			}
		})
	}
}

func TestDomesticOrderVenueAndTRAcrossLifecycle(t *testing.T) {
	t.Parallel()
	for _, market := range []string{"KRX", "NXT", "SOR"} {
		for _, sandbox := range []bool{false, true} {
			for _, side := range []broker.OrderSide{broker.OrderSideBuy, broker.OrderSideSell} {
				t.Run(market+"/"+string(side)+map[bool]string{false: "/real", true: "/sandbox"}[sandbox], func(t *testing.T) {
					a := newStubAdapter(t)
					a.sandbox = sandbox
					called := 0
					a.dispatcher.routes[kis.PathDomesticStockTradingOrderCash] = newEndpointRoute([]string{http.MethodPost}, func(_ context.Context, _, trID string, fields map[string]string) (any, error) {
						called++
						wantTR := "TTTC0012U"
						if side == broker.OrderSideSell {
							wantTR = "TTTC0011U"
						}
						if sandbox {
							wantTR = "V" + wantTR[1:]
						}
						if trID != wantTR || fields["EXCG_ID_DVSN_CD"] != market {
							t.Fatalf("place TR=%s fields=%v", trID, fields)
						}
						return map[string]any{"output": []any{map[string]string{"ODNO": "stub-1", "KRX_FWDG_ORD_ORGNO": "stub"}}}, nil
					})
					a.dispatcher.routes[kis.PathDomesticStockTradingOrderRvseCncl] = newEndpointRoute([]string{http.MethodPost}, func(_ context.Context, _, trID string, fields map[string]string) (any, error) {
						called++
						wantTR := "TTTC0013U"
						if sandbox {
							wantTR = "VTTC0013U"
						}
						if trID != wantTR || fields["EXCG_ID_DVSN_CD"] != market {
							t.Fatalf("modify/cancel TR=%s fields=%v", trID, fields)
						}
						return map[string]any{"output": []any{map[string]string{"ODNO": "stub-1", "KRX_FWDG_ORD_ORGNO": "stub"}}}, nil
					})
					result, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Market: market, Symbol: "005930", Side: side, Type: broker.OrderTypeLimit, Quantity: 1, Price: 70000})
					if sandbox && market != "KRX" {
						if !errors.Is(err, broker.ErrInvalidMarket) || result != nil || called != 0 {
							t.Fatalf("unsupported sandbox venue dispatched: result=%v err=%v calls=%d", result, err, called)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, err := a.ModifyOrder(context.Background(), result.OrderID, broker.ModifyOrderRequest{Quantity: 2, Price: 71000}); err != nil {
						t.Fatal(err)
					}
					if err := a.CancelOrder(context.Background(), result.OrderID); err != nil {
						t.Fatal(err)
					}
					if called != 3 {
						t.Fatalf("lifecycle calls=%d", called)
					}
				})
			}
		}
	}
}

func TestDomesticOrderRejectsUnknownVenue(t *testing.T) {
	t.Parallel()
	a := newStubAdapter(t)
	_, err := a.PlaceOrder(context.Background(), broker.OrderRequest{Market: "not-an-exchange", Symbol: "005930", Side: broker.OrderSideBuy, Type: broker.OrderTypeLimit, Quantity: 1, Price: 70000})
	if !errors.Is(err, broker.ErrInvalidMarket) {
		t.Fatalf("unknown market should fail before dispatch: %v", err)
	}
}
