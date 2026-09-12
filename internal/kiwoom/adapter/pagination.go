package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

const (
	maxAccountChartPages = 100
	maxAccountChartRows  = 100000
)

// collectEndpointRows is reserved for account/history reads. Raw endpoints and
// order operations always remain single-page calls. An incomplete traversal
// returns no rows, so callers cannot mistake partial data for a full result.
func (a *Adapter) collectEndpointRows(ctx context.Context, path, apiID string, request any, field string, maxPages int) ([]map[string]any, error) {
	return a.collectEndpointRowsUntil(ctx, path, apiID, request, field, maxPages, nil)
}

func (a *Adapter) collectEndpointRowsUntil(ctx context.Context, path, apiID string, request any, field string, maxPages int, stop func([]map[string]any) (bool, error)) ([]map[string]any, error) {
	var rows []map[string]any
	continuation := kiwoomspecs.Continuation{}
	seen := make(map[string]struct{})
	for page := 0; page < maxPages; page++ {
		result, err := a.CallEndpointPage(ctx, http.MethodPost, path, apiID, request, continuation)
		if err != nil {
			return nil, fmt.Errorf("kiwoom %s page %d: %w", apiID, page+1, err)
		}
		body, err := json.Marshal(result.Data)
		if err != nil {
			return nil, fmt.Errorf("encode Kiwoom %s page: %w", apiID, err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(body, &object); err != nil {
			return nil, fmt.Errorf("decode Kiwoom %s page: %w", apiID, err)
		}
		var pageRows []map[string]any
		if raw := object[field]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &pageRows); err != nil {
				return nil, fmt.Errorf("decode Kiwoom %s rows: %w", apiID, err)
			}
		}
		for _, row := range pageRows {
			if row == nil {
				return nil, fmt.Errorf("decode Kiwoom %s rows: null row", apiID)
			}
		}
		if len(rows)+len(pageRows) > maxAccountChartRows {
			return nil, fmt.Errorf("incomplete Kiwoom %s response: row limit exceeded", apiID)
		}
		rows = append(rows, pageRows...)
		if result.ContYN == "Y" {
			if _, exists := seen[result.NextKey]; exists {
				return nil, fmt.Errorf("incomplete Kiwoom %s response: repeated continuation key", apiID)
			}
			seen[result.NextKey] = struct{}{}
		}
		if stop != nil {
			done, err := stop(pageRows)
			if err != nil {
				return nil, err
			}
			if done {
				return rows, nil
			}
		}
		if result.ContYN != "Y" {
			return rows, nil
		}
		continuation = result.Continuation
	}
	return nil, fmt.Errorf("incomplete Kiwoom %s response: page limit exceeded", apiID)
}

// Chart APIs return newest bars first. Validate the full received page before
// stopping, and count only bars within the caller's requested date range.
// Reaching the lower date or result limit makes older pages unnecessary.
func chartPageStop(opts broker.OHLCVOpts) func([]map[string]any) (bool, error) {
	var previous time.Time
	lower, upper := chartDateBounds(opts)
	matched := 0
	return func(rows []map[string]any) (bool, error) {
		for _, row := range rows {
			date, ok := parseDateYYYYMMDDString(asAnyString(row["dt"]))
			if !ok {
				return false, fmt.Errorf("invalid Kiwoom chart bar date")
			}
			if !previous.IsZero() && !date.Before(previous) {
				return false, fmt.Errorf("kiwoom chart bars are not strictly descending")
			}
			previous = date
			if !lower.IsZero() && date.Before(lower) {
				continue
			}
			if !upper.IsZero() && date.After(upper) {
				continue
			}
			matched++
		}
		reachedLower := !lower.IsZero() && !previous.IsZero() && !previous.After(lower)
		return reachedLower || (opts.Limit > 0 && matched >= opts.Limit), nil
	}
}

func chartDateBounds(opts broker.OHLCVOpts) (lower, upper time.Time) {
	// Kiwoom bars and base_dt are calendar dates. Parsed bar dates use UTC;
	// preserve the caller's chosen calendar day instead of comparing offsets.
	day := func(value time.Time) time.Time {
		year, month, date := value.Date()
		return time.Date(year, month, date, 0, 0, 0, 0, time.UTC)
	}
	if !opts.From.IsZero() {
		lower = day(opts.From)
	}
	if !opts.To.IsZero() {
		upper = day(opts.To)
	}
	return lower, upper
}
