package kis

import "testing"

func TestNormalizeOverseasExchangeCode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"us":      "NASD",
		"nas":     "NASD",
		"nasd":    "NASD",
		"us-nyse": "NYSE",
		"amex":    "AMEX",
		"sehk":    "SEHK",
		"unknown": "UNKNOWN",
	}

	for in, want := range tests {
		got := normalizeOverseasExchangeCode(in)
		if got != want {
			t.Fatalf("normalizeOverseasExchangeCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveOverseasOrderTRID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exchange string
		side     string
		sandbox  bool
		want     string
	}{
		{exchange: "NASD", side: "buy", sandbox: false, want: "TTTT1002U"},
		{exchange: "NASD", side: "sell", sandbox: false, want: "TTTT1006U"},
		{exchange: "NYSE", side: "buy", sandbox: true, want: "VTTT1002U"},
		{exchange: "AMEX", side: "sell", sandbox: true, want: "VTTT1001U"},
		{exchange: "SEHK", side: "buy", sandbox: false, want: "TTTS1002U"},
		{exchange: "SEHK", side: "sell", sandbox: false, want: "TTTS1001U"},
	}

	for _, tc := range tests {
		got, err := ResolveOverseasOrderTRID(tc.exchange, tc.side, tc.sandbox)
		if err != nil {
			t.Fatalf("ResolveOverseasOrderTRID(%q,%q,%v) returned error: %v", tc.exchange, tc.side, tc.sandbox, err)
		}
		if got != tc.want {
			t.Fatalf("ResolveOverseasOrderTRID(%q,%q,%v) = %q, want %q", tc.exchange, tc.side, tc.sandbox, got, tc.want)
		}
	}
}

func TestResolveOverseasRvseCnclTRID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exchange string
		sandbox  bool
		want     string
	}{
		{exchange: "NASD", sandbox: false, want: "TTTT1004U"},
		{exchange: "NASDAQ", sandbox: true, want: "VTTT1004U"},
		{exchange: "SEHK", sandbox: false, want: "TTTS1003U"},
		{exchange: "SEHK", sandbox: true, want: "VTTS1003U"},
	}

	for _, tc := range tests {
		got, err := ResolveOverseasRvseCnclTRID(tc.exchange, tc.sandbox)
		if err != nil {
			t.Fatalf("ResolveOverseasRvseCnclTRID(%q,%v) returned error: %v", tc.exchange, tc.sandbox, err)
		}
		if got != tc.want {
			t.Fatalf("ResolveOverseasRvseCnclTRID(%q,%v) = %q, want %q", tc.exchange, tc.sandbox, got, tc.want)
		}
	}
}

func TestSingleTRID(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"TTTC8434R":                     "TTTC8434R",
		" FHKST01010100 ":               "FHKST01010100",
		"":                              "",
		"모의투자 미지원":                      "",
		"미지원":                           "",
		"(매도) TTTC0011U (매수) TTTC0012U": "",
		"(3개월이내) TTTC0081R (3개월이전) CTSC9215R": "",
	} {
		if got := singleTRID(in); got != want {
			t.Errorf("singleTRID(%q) = %q, want %q", in, got, want)
		}
	}
}
