package adapter

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

func TestToKISOverseasExchange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		market string
		code   string
		ok     bool
	}{
		{market: "US", code: "NASD", ok: true},
		{market: "us-nasdaq", code: "NASD", ok: true},
		{market: "NASDAQ", code: "NASD", ok: true},
		{market: "NYSE", code: "NYSE", ok: true},
		{market: "US-AMEX", code: "AMEX", ok: true},
		{market: "SEHK", code: "SEHK", ok: true},
		{market: "HK", code: "SEHK", ok: true},
		{market: "JP", code: "TKSE", ok: true},
		{market: "SH", code: "SHAA", ok: true},
		{market: "SZ", code: "SZAA", ok: true},
		{market: "HNX", code: "HASE", ok: true},
		{market: "HSX", code: "VNSE", ok: true},
		{market: "KRX", code: "", ok: false},
	}

	for _, tc := range tests {
		code, ok := toKISOverseasExchange(tc.market)
		if code != tc.code || ok != tc.ok {
			t.Fatalf("toKISOverseasExchange(%q) = (%q,%v), want (%q,%v)", tc.market, code, ok, tc.code, tc.ok)
		}
	}
}

func TestToKISOverseasQuoteExchange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		market string
		code   string
		ok     bool
	}{
		{market: "US", code: "NAS", ok: true},
		{market: "US-NYSE", code: "NYS", ok: true},
		{market: "US-AMEX", code: "AMS", ok: true},
		{market: "HK", code: "HKS", ok: true},
		{market: "JP", code: "TSE", ok: true},
		{market: "SH", code: "SHS", ok: true},
		{market: "SZ", code: "SZS", ok: true},
		{market: "HNX", code: "HNX", ok: true},
		{market: "HSX", code: "HSX", ok: true},
		{market: "KRX", code: "", ok: false},
	}

	for _, tc := range tests {
		code, ok := toKISOverseasQuoteExchange(tc.market)
		if code != tc.code || ok != tc.ok {
			t.Fatalf("toKISOverseasQuoteExchange(%q) = (%q,%v), want (%q,%v)", tc.market, code, ok, tc.code, tc.ok)
		}
	}
}

func TestToKISProductTypeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		market   string
		wantCode string
		wantOvr  bool
		wantErr  bool
	}{
		{market: "KRX", wantCode: "300", wantOvr: false},
		{market: "KOSDAQ", wantCode: "300", wantOvr: false},
		{market: "US", wantCode: "512", wantOvr: true},
		{market: "NYSE", wantCode: "513", wantOvr: true},
		{market: "AMEX", wantCode: "529", wantOvr: true},
		{market: "SEHK", wantCode: "501", wantOvr: true},
		{market: "UNKNOWN", wantErr: true},
	}

	for _, tc := range tests {
		gotCode, gotOvr, err := toKISProductTypeCode(tc.market)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("toKISProductTypeCode(%q) expected error", tc.market)
			}
			continue
		}
		if err != nil {
			t.Fatalf("toKISProductTypeCode(%q) unexpected error: %v", tc.market, err)
		}
		if gotCode != tc.wantCode || gotOvr != tc.wantOvr {
			t.Fatalf("toKISProductTypeCode(%q) = (%q,%v), want (%q,%v)", tc.market, gotCode, gotOvr, tc.wantCode, tc.wantOvr)
		}
	}
}

func TestToBrokerAssetType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		market string
		ptype  string
		want   string
	}{
		{market: "KOSPI", ptype: "stock", want: string(broker.AssetStock)},
		{market: "KOSPI", ptype: "etf", want: string(broker.AssetFund)},
		{market: "US-NASDAQ", ptype: "stock", want: string(broker.AssetOverseas)},
		{market: "US-NASDAQ", ptype: "etf", want: string(broker.AssetFund)},
	}

	for _, tc := range tests {
		got := toBrokerAssetType(tc.market, tc.ptype)
		if string(got) != tc.want {
			t.Fatalf("toBrokerAssetType(%q,%q) = %q, want %q", tc.market, tc.ptype, got, tc.want)
		}
	}
}

func TestApplyOHLCVOptions_FilterAndLimit(t *testing.T) {
	t.Parallel()

	src := []broker.OHLCV{
		{Timestamp: mustDate(t, "2026-02-05"), Open: 10, High: 12, Low: 9, Close: 11, Volume: 100},
		{Timestamp: mustDate(t, "2026-02-04"), Open: 11, High: 13, Low: 10, Close: 12, Volume: 200},
		{Timestamp: mustDate(t, "2026-02-03"), Open: 12, High: 14, Low: 11, Close: 13, Volume: 300},
	}

	out, err := applyOHLCVOptions(src, broker.OHLCVOpts{
		From:  mustDate(t, "2026-02-04"),
		To:    mustDate(t, "2026-02-05"),
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("applyOHLCVOptions() unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].Timestamp.Format("2006-01-02") != "2026-02-05" {
		t.Fatalf("unexpected timestamp: %s", out[0].Timestamp.Format("2006-01-02"))
	}
}

func TestApplyOHLCVOptions_WeeklyAggregation(t *testing.T) {
	t.Parallel()

	src := []broker.OHLCV{
		{Timestamp: mustDate(t, "2026-02-06"), Open: 12, High: 15, Low: 11, Close: 14, Volume: 120},
		{Timestamp: mustDate(t, "2026-02-05"), Open: 11, High: 14, Low: 10, Close: 12, Volume: 110},
		{Timestamp: mustDate(t, "2026-01-30"), Open: 9, High: 10, Low: 8, Close: 9, Volume: 90},
	}

	out, err := applyOHLCVOptions(src, broker.OHLCVOpts{Interval: "1w"})
	if err != nil {
		t.Fatalf("applyOHLCVOptions() unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].Volume != 230 {
		t.Fatalf("unexpected aggregated volume: %d", out[0].Volume)
	}
}

func TestApplyOHLCVOptions_UnsupportedInterval(t *testing.T) {
	t.Parallel()

	_, err := applyOHLCVOptions([]broker.OHLCV{}, broker.OHLCVOpts{Interval: "5m"})
	if err == nil {
		t.Fatal("expected error for unsupported interval")
	}
}

func TestOrderContextPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	orderDir := filepath.Join(tmpDir, "orders")

	a := NewAdapterWithOptions(false, "12345678-01", nil, orderDir, nil)
	now := time.Now().Truncate(time.Second)
	a.storeOrderContext("000001", orderContext{
		CANO:         "12345678",
		AccountPrdt:  "01",
		OrderID:      "000001",
		OrderOrgNo:   "06010",
		OrderDvsn:    "00",
		OrderQty:     3,
		OrderPrice:   10000,
		ExchangeCode: "KRX",
		Symbol:       "005930",
		Status:       broker.OrderStatusPending,
		UpdatedAt:    now,
	})

	b := NewAdapterWithOptions(false, "12345678-01", nil, orderDir, nil)
	got, ok := b.getOrderContext("000001")
	if !ok {
		t.Fatalf("persisted order context not loaded")
	}
	if got.Symbol != "005930" || got.OrderQty != 3 || got.OrderOrgNo != "06010" {
		t.Fatalf("unexpected loaded context: %+v", got)
	}
}

func TestToBrokerBalance_MapsExtendedFields(t *testing.T) {
	t.Parallel()

	summary := kisspecs.KISDomesticStockV1TradingInquireBalanceOutput2Item{
		DncaTotAmt:      "1000000",
		TotEvluAmt:      "1500000",
		PchsAmtSmtlAmt:  "1200000",
		EvluAmtSmtlAmt:  "1400000",
		EvluPflsSmtlAmt: "200000",
		AsstIcdcErngRt:  "16.67",
		NxdyExccAmt:     "50000",
		PrvsRcdlExccAmt: "12000",
		TotStlnSlngChgs: "300000",
	}

	got := toBrokerBalance("12345678-01", summary)

	if got.AccountID != "12345678-01" {
		t.Fatalf("AccountID = %q", got.AccountID)
	}
	if got.Cash != 1000000 {
		t.Fatalf("Cash = %v", got.Cash)
	}
	if got.BuyingPower != got.Cash {
		t.Fatalf("BuyingPower should match Cash: %v vs %v", got.BuyingPower, got.Cash)
	}
	if got.WithdrawableCash != got.Cash {
		t.Fatalf("WithdrawableCash should match Cash: %v vs %v", got.WithdrawableCash, got.Cash)
	}
	if got.TotalAssets != 1500000 {
		t.Fatalf("TotalAssets = %v", got.TotalAssets)
	}
	if got.PositionCost != 1200000 || got.PositionValue != 1400000 {
		t.Fatalf("position fields = (%v,%v)", got.PositionCost, got.PositionValue)
	}
	if got.SettlementT1 != 50000 || got.Unsettled != 12000 {
		t.Fatalf("settlement fields = (%v,%v)", got.SettlementT1, got.Unsettled)
	}
	if got.ReceivableAmount != 12000 {
		t.Fatalf("ReceivableAmount = %v", got.ReceivableAmount)
	}
	if got.LoanBalance != 300000 {
		t.Fatalf("LoanBalance = %v", got.LoanBalance)
	}
}

func TestToBrokerStockPosition_UsesPrprFallbackAndExtendedFields(t *testing.T) {
	t.Parallel()

	item := kisspecs.KISDomesticStockV1TradingInquireBalanceOutput1Item{
		Pdno:        "005930",
		PrdtName:    "삼성전자",
		HldgQty:     "10",
		OrdPsblQty:  "7",
		PchsAvgPric: "70000",
		Prpr:        "71000",
		PchsAmt:     "700000",
		EvluAmt:     "710000",
		EvluPflsAmt: "10000",
		EvluPflsRt:  "1.42",
	}

	got := toBrokerStockPosition(item)
	if got.Symbol != "005930" || got.Name != "삼성전자" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.Quantity != 10 || got.OrderableQty != 7 {
		t.Fatalf("unexpected qty fields: qty=%d orderable=%d", got.Quantity, got.OrderableQty)
	}
	if got.CurrentPrice != 71000 {
		t.Fatalf("CurrentPrice = %v", got.CurrentPrice)
	}
	if got.PurchaseValue != 700000 || got.MarketValue != 710000 {
		t.Fatalf("value fields = (%v,%v)", got.PurchaseValue, got.MarketValue)
	}
	if math.Abs(got.ProfitLossPct-1.42) > 1e-9 {
		t.Fatalf("ProfitLossPct = %v", got.ProfitLossPct)
	}
}

func TestParseFirstFloat(t *testing.T) {
	t.Parallel()

	if got := parseFirstFloat("", "bad", "123.45"); got != 123.45 {
		t.Fatalf("parseFirstFloat fallback = %v", got)
	}
	if got := parseFirstFloat("bad", ""); got != 0 {
		t.Fatalf("parseFirstFloat default = %v", got)
	}
}

func TestNormalizeISIN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "kr7005930003", want: "KR7005930003"},
		{in: "US0378331005", want: "US0378331005"},
		{in: "005930", want: ""},
		{in: "KR70-05930003", want: ""},
	}

	for _, tc := range tests {
		if got := normalizeISIN(tc.in); got != tc.want {
			t.Fatalf("normalizeISIN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return v
}
