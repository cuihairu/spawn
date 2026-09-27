package client

import (
	"testing"
	"time"
)

// newClockBreaker 创建带可推进时钟的熔断器。
func newClockBreaker(t *testing.T, maxFailures int, cooldown time.Duration) (*Breaker, func(time.Duration)) {
	t.Helper()
	return fakeClockBreaker(maxFailures, cooldown)
}

func TestBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	b, _ := newClockBreaker(t, 3, time.Minute)

	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatalf("call %d: expected closed breaker to allow", i+1)
		}
		b.Failure()
	}
	if b.stateValue() != stateClosed {
		t.Fatalf("expected closed after 2 failures, got %v", b.stateValue())
	}

	b.Failure()
	if b.stateValue() != stateOpen {
		t.Fatalf("expected open after 3 consecutive failures, got %v", b.stateValue())
	}
}

func TestBreaker_OpenRejectsUntilCooldownElapses(t *testing.T) {
	t.Parallel()

	b, advance := newClockBreaker(t, 1, time.Minute)

	b.Failure() // 立即 open
	if b.stateValue() != stateOpen {
		t.Fatalf("expected open, got %v", b.stateValue())
	}

	for i := 0; i < 5; i++ {
		if b.Allow() {
			t.Fatalf("attempt %d: expected open breaker to reject", i+1)
		}
	}

	// 冷却差 1ns 仍拒绝。
	advance(time.Minute - time.Nanosecond)
	if b.Allow() {
		t.Fatal("expected rejection 1ns before cooldown elapses")
	}

	// 冷却结束：进入半开并放行探测。
	advance(time.Nanosecond)
	if !b.Allow() {
		t.Fatal("expected half-open breaker to allow probe")
	}
	if b.stateValue() != stateHalfOpen {
		t.Fatalf("expected half-open, got %v", b.stateValue())
	}
}

func TestBreaker_HalfOpenAllowsSingleProbe(t *testing.T) {
	t.Parallel()

	b, advance := newClockBreaker(t, 1, time.Minute)
	b.Failure()
	advance(time.Minute)

	if !b.Allow() {
		t.Fatal("expected probe to be allowed")
	}
	if b.Allow() {
		t.Fatal("expected only a single probe in half-open")
	}
	if b.Allow() {
		t.Fatal("expected additional probes to be rejected")
	}
}

func TestBreaker_HalfOpenSuccessCloses(t *testing.T) {
	t.Parallel()

	b, advance := newClockBreaker(t, 3, time.Minute)

	b.Failure()
	b.Failure()
	b.Failure()
	advance(time.Minute)

	if !b.Allow() {
		t.Fatal("expected probe to be allowed")
	}
	b.Success()
	if b.stateValue() != stateClosed {
		t.Fatalf("expected closed after probe success, got %v", b.stateValue())
	}

	// 恢复 closed 后失败计数已复位：需要再次达到阈值才会熔断。
	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatalf("recovered call %d: expected allow", i+1)
		}
		b.Failure()
	}
	if b.stateValue() != stateClosed {
		t.Fatalf("expected still closed after 2 post-recovery failures, got %v", b.stateValue())
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	t.Parallel()

	b, advance := newClockBreaker(t, 1, time.Minute)

	b.Failure() // open
	advance(time.Minute)

	if !b.Allow() {
		t.Fatal("expected probe to be allowed")
	}
	b.Failure()
	if b.stateValue() != stateOpen {
		t.Fatalf("expected re-open after failed probe, got %v", b.stateValue())
	}

	// 重新进入冷却期。
	if b.Allow() {
		t.Fatal("expected rejection during new cooldown")
	}
	advance(time.Minute)
	if !b.Allow() {
		t.Fatal("expected probe after new cooldown")
	}
}

func TestBreaker_SuccessResetsFailureStreak(t *testing.T) {
	t.Parallel()

	b, _ := newClockBreaker(t, 3, time.Minute)

	b.Failure()
	b.Failure()
	b.Success()
	if b.stateValue() != stateClosed {
		t.Fatalf("expected closed, got %v", b.stateValue())
	}

	b.Failure()
	b.Failure()
	if b.stateValue() != stateClosed {
		t.Fatalf("expected closed after streak reset + 2 failures, got %v", b.stateValue())
	}

	b.Failure()
	if b.stateValue() != stateOpen {
		t.Fatalf("expected open after 3 consecutive failures post-reset, got %v", b.stateValue())
	}
}

func TestBreaker_GuardsInvalidConfig(t *testing.T) {
	t.Parallel()

	// maxFailures=0 按 1 处理；cooldown 为负按 0 处理（立即半开）。
	b := NewBreaker(0, -time.Second)

	if b.maxFailures != 1 {
		t.Fatalf("expected maxFailures clamped to 1, got %d", b.maxFailures)
	}

	b.Failure()
	if b.stateValue() != stateOpen {
		t.Fatalf("expected open after single failure, got %v", b.stateValue())
	}
	if !b.Allow() {
		t.Fatal("expected immediate half-open with zero cooldown")
	}
}
