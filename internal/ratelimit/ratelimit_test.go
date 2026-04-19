package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestWait_respectsRate(t *testing.T) {
	// 10 req/s, burst 1 → 각 요청 사이 ~100ms
	lim := New("test", 10, 1)

	ctx := context.Background()
	// 첫 요청은 burst로 즉시 통과
	if err := lim.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := lim.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	// 5 requests at 10/s = ~500ms 이상 걸려야 함
	if elapsed < 400*time.Millisecond {
		t.Errorf("too fast: %v (expected >= 400ms)", elapsed)
	}
}

func TestWait_burst(t *testing.T) {
	// 5 req/s, burst 5 → 첫 5개는 즉시 통과
	lim := New("test-burst", 5, 5)

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := lim.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	// burst 5개는 거의 즉시 통과해야 함
	if elapsed > 50*time.Millisecond {
		t.Errorf("burst too slow: %v (expected < 50ms)", elapsed)
	}

	// 6번째는 대기해야 함
	start = time.Now()
	if err := lim.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed = time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("6th request too fast: %v (expected >= 100ms)", elapsed)
	}
}

func TestWait_cancelledContext(t *testing.T) {
	// 1 req/s, burst 1
	lim := New("test-cancel", 1, 1)

	ctx := context.Background()
	// 첫 요청으로 burst 소진
	_ = lim.Wait(ctx)

	// 취소된 context로 호출하면 에러
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := lim.Wait(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestShared_sameKeyReturnsSameLimiter(t *testing.T) {
	a := Shared("test-shared", 5, 1, "key-alpha")
	b := Shared("test-shared", 5, 1, "key-alpha")
	c := Shared("test-shared", 5, 1, "key-beta")

	if a != b {
		t.Fatal("Shared with identical (name, key) must return the same *Limiter")
	}
	if a == c {
		t.Fatal("Shared with different keys must return distinct *Limiter")
	}
}

func TestShared_serializesIndependentCallers(t *testing.T) {
	// Two callers that look up the shared limiter independently must
	// collectively stay under the configured rate (this is the bug
	// that lets per-client limiters overshoot the upstream quota).
	a := Shared("test-coop", 10, 1, "shared-key")
	b := Shared("test-coop", 10, 1, "shared-key")

	ctx := context.Background()
	if err := a.Wait(ctx); err != nil { // drain burst
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < 4; i++ {
		lim := a
		if i%2 == 1 {
			lim = b
		}
		if err := lim.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	// 4 req at 10/s across two handles = ~400ms of waiting.
	if elapsed < 300*time.Millisecond {
		t.Errorf("independent callers bypassed shared rate: %v (expected >= 300ms)", elapsed)
	}
}

func TestAllow(t *testing.T) {
	lim := New("test-allow", 1, 1)

	// 첫 번째는 통과
	if !lim.Allow() {
		t.Error("first Allow() should return true")
	}

	// 즉시 두 번째는 거부
	if lim.Allow() {
		t.Error("second Allow() should return false")
	}
}
