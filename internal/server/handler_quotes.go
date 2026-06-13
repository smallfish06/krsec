package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

// handleGetQuote handles GET /quotes/{market}/{symbol}
func (s *Server) handleGetQuote(c fuego.ContextNoBody) (Response, error) {
	market := c.PathParam("market")
	symbol := c.PathParam("symbol")
	accountID := strings.TrimSpace(c.QueryParam("account_id"))

	var brk broker.Broker
	if accountID != "" {
		var status int
		var reason string
		brk, status, reason = s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return respond(c, status, Response{
				OK:    false,
				Error: reason,
			})
		}
	} else {
		brk = s.getFirstBroker()
	}
	if brk == nil {
		return respond(c, http.StatusInternalServerError, Response{
			OK:    false,
			Error: "no broker available",
		})
	}

	quote, err := brk.GetQuote(c.Context(), market, symbol)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:    false,
			Error: err.Error(),
		})
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   quote,
		Broker: brk.Name(),
	})
}

// handleGetOHLCV handles GET /quotes/{market}/{symbol}/ohlcv
func (s *Server) handleGetOHLCV(c fuego.ContextNoBody) (Response, error) {
	market := c.PathParam("market")
	symbol := c.PathParam("symbol")
	accountID := strings.TrimSpace(c.QueryParam("account_id"))

	var brk broker.Broker
	if accountID != "" {
		var status int
		var reason string
		brk, status, reason = s.resolveBrokerByAccountID(accountID)
		if brk == nil {
			return respond(c, status, Response{
				OK:    false,
				Error: reason,
			})
		}
	} else {
		brk = s.getFirstBroker()
	}
	if brk == nil {
		return respond(c, http.StatusInternalServerError, Response{
			OK:    false,
			Error: "no broker available",
		})
	}

	opts, err := parseOHLCVOpts(c)
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{
			OK:    false,
			Error: err.Error(),
		})
	}
	ohlcv, err := brk.GetOHLCV(c.Context(), market, symbol, opts)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusInternalServerError), Response{
			OK:    false,
			Error: err.Error(),
		})
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   ohlcv,
		Broker: brk.Name(),
	})
}

type queryReader interface {
	QueryParam(name string) string
}

func parseOHLCVOpts(c queryReader) (broker.OHLCVOpts, error) {
	opts := broker.OHLCVOpts{
		Interval: c.QueryParam("interval"),
		Limit:    100,
	}
	if strings.TrimSpace(opts.Interval) == "" {
		opts.Interval = "1d"
	}
	switch strings.ToLower(strings.TrimSpace(opts.Interval)) {
	case "1m", "m", "min", "minute", "1d", "d", "day", "daily", "1w", "w", "week", "weekly", "1mo", "mo", "month", "monthly":
	default:
		return broker.OHLCVOpts{}, fmt.Errorf("invalid interval: %s", opts.Interval)
	}

	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return broker.OHLCVOpts{}, fmt.Errorf("invalid limit: %s", raw)
		}
		if limit > 2000 {
			limit = 2000
		}
		opts.Limit = limit
	}

	if raw := strings.TrimSpace(c.QueryParam("from")); raw != "" {
		t, err := parseDateParam(raw)
		if err != nil {
			return broker.OHLCVOpts{}, fmt.Errorf("invalid from date: %s", raw)
		}
		opts.From = t
	}
	if raw := strings.TrimSpace(c.QueryParam("to")); raw != "" {
		t, err := parseDateParam(raw)
		if err != nil {
			return broker.OHLCVOpts{}, fmt.Errorf("invalid to date: %s", raw)
		}
		opts.To = t
	}
	if !opts.From.IsZero() && !opts.To.IsZero() && opts.From.After(opts.To) {
		return broker.OHLCVOpts{}, fmt.Errorf("from date must be before to date")
	}
	return opts, nil
}

func parseDateParam(v string) (time.Time, error) {
	layouts := []string{
		"2006-01-02",
		"20060102",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format")
}
