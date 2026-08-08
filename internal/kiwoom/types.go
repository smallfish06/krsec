package kiwoom

import (
	"strconv"
	"strings"
)

// StockOrderSide indicates buy/sell for order placement.
type StockOrderSide string

const (
	StockOrderSideBuy  StockOrderSide = "buy"
	StockOrderSideSell StockOrderSide = "sell"
)

func asString(v any) string {
	s := strings.TrimSpace(toString(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func asFloat64(v any) float64 {
	s := asString(v)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func asInt64(v any) int64 {
	s := asString(v)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return n
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func normalizeSymbolCode(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	return strings.TrimPrefix(s, "A")
}
