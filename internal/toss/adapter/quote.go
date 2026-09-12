package adapter

import (
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/toss"
	"github.com/smallfish06/krsec/pkg/broker"
)

func applyQuoteCandles(quote *broker.Quote, quoteTime time.Time, currency string, candles []toss.Candle) {
	zone := ""
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "KRW":
		zone = "Asia/Seoul"
	case "USD":
		zone = "America/New_York"
	default:
		return
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return
	}
	quoteDate := quoteTime.In(location).Format(time.DateOnly)
	previousDate := ""
	for _, candle := range candles {
		stamp := parseTime(candle.Timestamp)
		if stamp.IsZero() || !strings.EqualFold(strings.TrimSpace(candle.Currency), strings.TrimSpace(currency)) {
			continue
		}
		date := stamp.In(location).Format(time.DateOnly)
		switch {
		case date == quoteDate:
			quote.Open = parseDecimal(candle.OpenPrice)
			quote.High = parseDecimal(candle.HighPrice)
			quote.Low = parseDecimal(candle.LowPrice)
			quote.Close = parseDecimal(candle.ClosePrice)
			quote.Volume = parseDecimalInt64(candle.Volume)
		case date < quoteDate && date > previousDate:
			previousDate = date
			quote.PrevClose = parseDecimal(candle.ClosePrice)
		}
	}
	if quote.PrevClose > 0 {
		quote.Change = quote.Price - quote.PrevClose
		quote.ChangeRate = quote.Change / quote.PrevClose * 100
	}
}
