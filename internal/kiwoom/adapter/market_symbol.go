package adapter

import (
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
)

func stripVenueSuffix(symbol string) string {
	return strings.TrimSuffix(strings.TrimSuffix(symbol, "_NX"), "_AL")
}

// Market data selects a venue through the stock code, unlike order APIs which
// use dmst_stex_tp. Explicit conflicting market/suffix inputs fail locally.
func marketDataSymbol(market, symbol string) (request, plain, outputMarket string, err error) {
	exchange, err := toKiwoomExchange(market)
	if err != nil {
		return "", "", "", err
	}
	symbol = normalizeSymbol(symbol)
	plain = stripVenueSuffix(symbol)
	if plain == "" || strings.Contains(plain, "_") {
		return "", "", "", broker.ErrInvalidSymbol
	}
	suffixExchange := ""
	switch {
	case strings.HasSuffix(symbol, "_NX"):
		suffixExchange = "NXT"
	case strings.HasSuffix(symbol, "_AL"):
		suffixExchange = "SOR"
	}
	if suffixExchange != "" {
		if strings.TrimSpace(market) == "" {
			exchange = suffixExchange
		} else if exchange != suffixExchange {
			return "", "", "", fmt.Errorf("%w: symbol suffix conflicts with market %s", broker.ErrInvalidMarket, market)
		}
	}
	request = plain
	switch exchange {
	case "NXT":
		request += "_NX"
	case "SOR":
		request += "_AL"
	}
	return request, plain, exchange, nil
}
