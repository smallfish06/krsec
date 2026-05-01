package server

import (
	"context"
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
}

type kisProxyRateLimiter struct {
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
	return &kisProxyRateLimiter{
		limiter: rate.NewLimiter(rate.Limit(opts.RequestsPerSecond), opts.Burst),
	}
}

func kisProxyRateLimitOptionsFromConfig(cfg config.KISProxyRateLimitConfig) KISProxyRateLimitOptions {
	opts := KISProxyRateLimitOptions{
		RequestsPerSecond: cfg.RequestsPerSecond,
		Burst:             cfg.Burst,
	}
	if cfg.Enabled != nil {
		opts.Disabled = !*cfg.Enabled
	}
	return opts
}

func (opts KISProxyRateLimitOptions) isZero() bool {
	return !opts.Disabled && opts.RequestsPerSecond == 0 && opts.Burst == 0
}

func (l *kisProxyRateLimiter) wait(ctx context.Context) (time.Duration, error) {
	if l == nil || l.limiter == nil {
		return 0, nil
	}
	start := time.Now()
	if err := l.limiter.Wait(ctx); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}
