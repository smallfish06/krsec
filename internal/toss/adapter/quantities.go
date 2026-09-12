package adapter

import (
	"math/big"
	"strings"
)

// parseQuantity accepts the nonnegative decimal strings used by Toss. Avoid
// float64 so fractional executions and large whole quantities stay exact.
func parseQuantity(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 30 {
		return nil, false
	}
	digits, dots := 0, 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.':
			dots++
		default:
			return nil, false
		}
	}
	if digits == 0 || dots > 1 {
		return nil, false
	}
	return new(big.Rat).SetString(value)
}

func remainingQuantity(quantity, filled string) string {
	q, qOK := parseQuantity(quantity)
	f, fOK := parseQuantity(filled)
	if !qOK || !fOK {
		return ""
	}
	r := new(big.Rat).Sub(q, f)
	if r.Sign() <= 0 {
		return "0"
	}
	scale := max(quantityScale(quantity), quantityScale(filled))
	value := r.FloatString(scale)
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return value
}

func quantityScale(value string) int {
	_, fraction, found := strings.Cut(strings.TrimSpace(value), ".")
	if !found {
		return 0
	}
	return len(fraction)
}
