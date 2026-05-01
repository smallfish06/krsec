package server

import (
	"context"
	"testing"
	"time"
)

func TestKISProxyRateLimiterWaitsBetweenBurstOneCalls(t *testing.T) {
	limiter := newKISProxyRateLimiter(KISProxyRateLimitOptions{
		RequestsPerSecond: 20,
		Burst:             1,
	})

	if _, err := limiter.wait(context.Background()); err != nil {
		t.Fatalf("first wait unexpected error: %v", err)
	}

	start := time.Now()
	if _, err := limiter.wait(context.Background()); err != nil {
		t.Fatalf("second wait unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("second wait elapsed = %s, want at least 40ms", elapsed)
	}
}

func TestKISProxyRateLimiterDisabledDoesNotWait(t *testing.T) {
	limiter := newKISProxyRateLimiter(KISProxyRateLimitOptions{Disabled: true})
	if limiter != nil {
		t.Fatalf("disabled limiter = %#v, want nil", limiter)
	}
}
