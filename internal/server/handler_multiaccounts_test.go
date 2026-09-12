package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

func TestHandleAccountsSummary_ReturnsServiceUnavailableWhenAllBalancesFail(t *testing.T) {
	t.Parallel()

	b := newMockBroker(t, "KIS")
	b.On("GetBalance", mock.Anything, "acc1").Return((*broker.Balance)(nil), errors.New("upstream unavailable")).Once()

	s := newOrderTestServer(
		map[string]broker.Broker{"acc1": b},
		[]config.AccountConfig{{AccountID: "acc1"}},
	)

	req := httptest.NewRequest(http.MethodGet, "/accounts/summary", nil)
	rr := performFiberRequest(t, s, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if resp.OK {
		t.Fatalf("expected ok=false")
	}
	if resp.Error != "failed to retrieve balances from all accounts" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestHandleAccountsSummary_DoesNotPublishIncompleteTotals(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"error", "nil balance", "unavailable total", "unavailable cash"} {
		t.Run(scenario, func(t *testing.T) {
			good := newMockBroker(t, "KIS")
			good.On("GetBalance", mock.Anything, "good").Return(&broker.Balance{AccountID: "good", TotalAssets: 100, Cash: 10}, nil).Once()
			bad := newMockBroker(t, "TOSS")
			var balance *broker.Balance
			var err error
			switch scenario {
			case "error":
				err = errors.New("upstream unavailable")
			case "unavailable total":
				balance = &broker.Balance{UnavailableFields: []string{"total_assets"}}
			case "unavailable cash":
				balance = &broker.Balance{UnavailableFields: []string{"cash"}}
			}
			bad.On("GetBalance", mock.Anything, "bad").Return(balance, err).Once()
			s := newOrderTestServer(map[string]broker.Broker{"good": good, "bad": bad}, []config.AccountConfig{{AccountID: "good"}, {AccountID: "bad"}})
			rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodGet, "/accounts/summary", nil))
			resp := decodeResponse(t, rr)
			if rr.Code != http.StatusServiceUnavailable || resp.OK || resp.Data != nil {
				t.Fatalf("partial total published: status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleAccountsSummary_CompleteBalancesIncludingZero(t *testing.T) {
	b := newMockBroker(t, "KIS")
	b.On("GetBalance", mock.Anything, "acc1").Return(&broker.Balance{AccountID: "acc1"}, nil).Once()
	s := newOrderTestServer(map[string]broker.Broker{"acc1": b}, []config.AccountConfig{{AccountID: "acc1"}})
	rr := performFiberRequest(t, s, httptest.NewRequest(http.MethodGet, "/accounts/summary", nil))
	if rr.Code != http.StatusOK || !decodeResponse(t, rr).OK {
		t.Fatalf("observed zero balance rejected: %s", rr.Body.String())
	}
}
