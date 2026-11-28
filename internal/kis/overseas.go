package kis

import (
	"fmt"
	"strings"
)

func normalizeOverseasExchangeCode(exchangeCode string) string {
	code := strings.ToUpper(strings.TrimSpace(exchangeCode))
	switch code {
	case "US", "NAS", "NASD", "NASDAQ", "US-NASDAQ":
		return "NASD"
	case "NYS", "NYSE", "US-NYSE":
		return "NYSE"
	case "AMS", "AMEX", "US-AMEX":
		return "AMEX"
	default:
		return code
	}
}

func overseasOrderTRID(exchangeCode, side string, sandbox bool) (string, error) {
	code := normalizeOverseasExchangeCode(exchangeCode)
	var trID string
	switch strings.ToLower(side) {
	case "buy":
		switch code {
		case "NASD", "NYSE", "AMEX":
			trID = "TTTT1002U"
		case "SEHK":
			trID = "TTTS1002U"
		case "SHAA":
			trID = "TTTS0202U"
		case "SZAA":
			trID = "TTTS0305U"
		case "TKSE":
			trID = "TTTS0308U"
		case "HASE", "VNSE":
			trID = "TTTS0311U"
		default:
			return "", fmt.Errorf("unsupported overseas exchange code: %s", exchangeCode)
		}
	case "sell":
		switch code {
		case "NASD", "NYSE", "AMEX":
			trID = "TTTT1006U"
		case "SEHK":
			trID = "TTTS1001U"
		case "SHAA":
			trID = "TTTS1005U"
		case "SZAA":
			trID = "TTTS0304U"
		case "TKSE":
			trID = "TTTS0307U"
		case "HASE", "VNSE":
			trID = "TTTS0310U"
		default:
			return "", fmt.Errorf("unsupported overseas exchange code: %s", exchangeCode)
		}
	default:
		return "", fmt.Errorf("unsupported order side: %s", side)
	}

	if sandbox {
		// Paper trading mirrors the real ID with a V prefix, except the US
		// sell order, which KIS documents as VTTT1001U.
		if trID == "TTTT1006U" {
			return "VTTT1001U", nil
		}
		trID = "V" + trID[1:]
	}
	return trID, nil
}

// ResolveOverseasOrderTRID returns documented TR_ID for overseas order endpoints.
func ResolveOverseasOrderTRID(exchangeCode, side string, sandbox bool) (string, error) {
	return overseasOrderTRID(exchangeCode, side, sandbox)
}

func overseasRvseCnclTRID(exchangeCode string, sandbox bool) (string, error) {
	code := normalizeOverseasExchangeCode(exchangeCode)
	var trID string
	switch code {
	case "NASD", "NYSE", "AMEX":
		trID = "TTTT1004U"
	case "SEHK":
		trID = "TTTS1003U"
	case "SHAA":
		trID = "TTTS0302U"
	case "SZAA":
		trID = "TTTS0306U"
	case "TKSE":
		trID = "TTTS0309U"
	case "HASE", "VNSE":
		trID = "TTTS0312U"
	default:
		return "", fmt.Errorf("unsupported overseas exchange code: %s", exchangeCode)
	}

	if sandbox && len(trID) > 0 {
		trID = "V" + trID[1:]
	}
	return trID, nil
}

// ResolveOverseasRvseCnclTRID returns documented TR_ID for overseas revise/cancel endpoints.
func ResolveOverseasRvseCnclTRID(exchangeCode string, sandbox bool) (string, error) {
	return overseasRvseCnclTRID(exchangeCode, sandbox)
}
