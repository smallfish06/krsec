package server

import (
	"context"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/smallfish06/krsec/pkg/config"
)

const (
	defaultKISProxyRequestsPerSecond = 15
	defaultKISProxyRateLimitBurst    = 1
)

// KISProxyRateLimitOptions configures outbound KIS upstream throttling.
type KISProxyRateLimitOptions struct {
	Disabled          bool
	RequestsPerSecond float64
	Burst             int
	Overrides         []KISProxyRateLimitOverrideOptions
}

type KISProxyRateLimitOverrideOptions struct {
	Path              string
	TRID              string
	RequestsPerSecond float64
	Burst             int
}

type kisProxyRateLimiter struct {
	limiter   *rate.Limiter
	overrides []kisProxyRateLimitOverride
}

type kisProxyRateLimitOverride struct {
	path    string
	trID    string
	limiter *rate.Limiter
}

func newKISProxyRateLimiter(opts KISProxyRateLimitOptions) *kisProxyRateLimiter {
	if opts.Disabled {
		return nil
	}
	if opts.RequestsPerSecond <= 0 {
		opts.RequestsPerSecond = defaultKISProxyRequestsPerSecond
	}
	if opts.Burst <= 0 {
		opts.Burst = defaultKISProxyRateLimitBurst
	}
	limiter := &kisProxyRateLimiter{
		limiter: newLimiter(opts.RequestsPerSecond, opts.Burst),
	}
	for _, override := range opts.Overrides {
		path := normalizeKISProxyPath(override.Path)
		if path == "" {
			continue
		}
		requestsPerSecond := override.RequestsPerSecond
		if requestsPerSecond <= 0 {
			requestsPerSecond = opts.RequestsPerSecond
		}
		burst := override.Burst
		if burst <= 0 {
			burst = opts.Burst
		}
		limiter.overrides = append(limiter.overrides, kisProxyRateLimitOverride{
			path:    path,
			trID:    strings.ToUpper(strings.TrimSpace(override.TRID)),
			limiter: newLimiter(requestsPerSecond, burst),
		})
	}
	return limiter
}

func kisProxyRateLimitOptionsFromConfig(cfg config.KISProxyRateLimitConfig) KISProxyRateLimitOptions {
	opts := KISProxyRateLimitOptions{
		RequestsPerSecond: cfg.RequestsPerSecond,
		Burst:             cfg.Burst,
	}
	for _, override := range cfg.Overrides {
		opts.Overrides = append(opts.Overrides, KISProxyRateLimitOverrideOptions{
			Path:              override.Path,
			TRID:              override.TRID,
			RequestsPerSecond: override.RequestsPerSecond,
			Burst:             override.Burst,
		})
	}
	if cfg.Enabled != nil {
		opts.Disabled = !*cfg.Enabled
	}
	return opts
}

func (opts KISProxyRateLimitOptions) isZero() bool {
	return !opts.Disabled && opts.RequestsPerSecond == 0 && opts.Burst == 0 && len(opts.Overrides) == 0
}

func newLimiter(requestsPerSecond float64, burst int) *rate.Limiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = defaultKISProxyRequestsPerSecond
	}
	if burst <= 0 {
		burst = defaultKISProxyRateLimitBurst
	}
	return rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
}

func (l *kisProxyRateLimiter) wait(ctx context.Context, path string, trID string) (time.Duration, error) {
	if l == nil || l.limiter == nil {
		return 0, nil
	}
	start := time.Now()
	if limiter := l.matchOverride(path, trID); limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return time.Since(start), err
		}
	}
	if err := l.limiter.Wait(ctx); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}

func (l *kisProxyRateLimiter) matchOverride(path string, trID string) *rate.Limiter {
	if l == nil {
		return nil
	}
	path = normalizeKISProxyPath(path)
	trID = strings.ToUpper(strings.TrimSpace(trID))

	var pathOnly *rate.Limiter
	for _, override := range l.overrides {
		if override.path != path {
			continue
		}
		if override.trID == "" {
			if pathOnly == nil {
				pathOnly = override.limiter
			}
			continue
		}
		if override.trID == trID {
			return override.limiter
		}
	}
	return pathOnly
}
