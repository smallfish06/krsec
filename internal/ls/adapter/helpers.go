package adapter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

func lsExchangeCode(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "", "KRX", "KOSPI", "KOSDAQ", "K":
		return "K"
	case "NXT", "N":
		return "N"
	case "U", "UNIFIED", "ALL":
		return "U"
	default:
		return ""
	}
}

func lsOverseasExchangeCode(market string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US", "US-NASDAQ", "NASDAQ", "NAS", "NASD", "XNAS":
		return "82", true
	case "US-NYSE", "NYSE", "NYS", "XNYS", "US-AMEX", "AMEX", "AMS", "XASE":
		return "81", true
	default:
		return "", false
	}
}

func lsOverseasExchangeGroup(market string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US", "US-NASDAQ", "NASDAQ", "NAS", "NASD", "XNAS":
		return "2", true
	case "US-NYSE", "NYSE", "NYS", "XNYS", "US-AMEX", "AMEX", "AMS", "XASE":
		return "1", true
	default:
		return "", false
	}
}

func lsMarketMember(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "NXT", "N":
		return "NXT"
	default:
		return "KRX"
	}
}

func lsOrderSide(side broker.OrderSide) (string, error) {
	switch side {
	case broker.OrderSideSell:
		return "1", nil
	case broker.OrderSideBuy:
		return "2", nil
	default:
		return "", fmt.Errorf("%w: unsupported LS order side %q", broker.ErrInvalidOrderRequest, side)
	}
}

func lsOrderType(orderType broker.OrderType) (string, error) {
	switch orderType {
	case broker.OrderTypeLimit:
		return "00", nil
	case broker.OrderTypeMarket:
		return "03", nil
	default:
		return "", fmt.Errorf("%w: unsupported LS order type %q", broker.ErrInvalidOrderRequest, orderType)
	}
}

func normalizeSymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	symbol = strings.TrimPrefix(symbol, "A")
	return symbol
}

func normalizeOverseasSymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	if len(symbol) > 2 && (strings.HasPrefix(symbol, "81") || strings.HasPrefix(symbol, "82")) {
		return strings.TrimSpace(symbol[2:])
	}
	return symbol
}

func normalizeOrderSymbol(symbol string, sandbox bool) string {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return ""
	}
	if sandbox && len(symbol) == 6 {
		return "A" + symbol
	}
	return symbol
}

func normalizeOutputMarket(input string, row map[string]interface{}) string {
	if jang := strings.ToUpper(strings.TrimSpace(anyString(row["janginfo"]))); jang != "" {
		return jang
	}
	switch strings.ToUpper(strings.TrimSpace(input)) {
	case "N", "NXT":
		return "NXT"
	case "U", "UNIFIED", "ALL":
		return "UNIFIED"
	default:
		return "KRX"
	}
}

func normalizeOverseasOutputMarket(input string, row map[string]interface{}) string {
	switch strings.TrimSpace(anyString(row["exchcd"])) {
	case "82":
		return "US-NASDAQ"
	case "81":
		switch strings.ToUpper(strings.TrimSpace(input)) {
		case "US-AMEX", "AMEX", "AMS", "XASE":
			return "US-AMEX"
		case "US-NYSE", "NYSE", "NYS", "XNYS":
			return "US-NYSE"
		default:
			return "US"
		}
	default:
		if input = strings.ToUpper(strings.TrimSpace(input)); input != "" {
			return input
		}
		return "US"
	}
}

func normalizeOverseasExchangeName(input string, row map[string]interface{}) string {
	if name := strings.TrimSpace(anyString(row["exchange_name"])); name != "" {
		return name
	}
	switch strings.TrimSpace(anyString(row["exchcd"])) {
	case "82":
		return "NASDAQ"
	case "81":
		switch strings.ToUpper(strings.TrimSpace(input)) {
		case "US-AMEX", "AMEX", "AMS", "XASE":
			return "AMEX"
		default:
			return "NYSE/AMEX"
		}
	default:
		return ""
	}
}

func lsSignedDiff(value float64, sign string) float64 {
	switch strings.TrimSpace(sign) {
	case "4", "5":
		if value > 0 {
			return -value
		}
	}
	return value
}

func normalizePositionMarket(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "2", "KOSDAQ":
		return "KOSDAQ"
	case "1", "KOSPI":
		return "KOSPI"
	case "N", "NXT":
		return "NXT"
	default:
		return "KRX"
	}
}

func marketFromMasterCode(code string) string {
	switch strings.TrimSpace(code) {
	case "1":
		return "KOSPI"
	case "2":
		return "KOSDAQ"
	default:
		return "KRX"
	}
}

func mapValue(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	row, ok := v.(map[string]interface{})
	return row, ok
}

func anyString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func anyFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := strconv.ParseFloat(t.String(), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(t), ",", ""), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(fmt.Sprint(t)), ",", ""), 64)
		return f
	}
}

func anyInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		i, _ := strconv.ParseInt(t.String(), 10, 64)
		return i
	case string:
		i, _ := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(t), ",", ""), 10, 64)
		return i
	default:
		i, _ := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(fmt.Sprint(t)), ",", ""), 10, 64)
		return i
	}
}

func parseYYYYMMDD(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102", v)
	return t, err == nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}
