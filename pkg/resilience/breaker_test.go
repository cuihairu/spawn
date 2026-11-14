package resilience

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// newClockBreaker 创建带可推进时钟的熔断器。
func newClockBreaker(t *testing.T, maxFailures int, cooldown time.Duration) (*Breaker, func(time.Duration)) {
	t.Helper()
	now := time.Now()
	b := NewBreaker(maxFailures, cooldown)
	b.SetNow(func() time.Time { return now })
	return b, func(d time.Duration) { now = now.Add(d) }
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
	if b.State() != StateClosed {
		t.Fatalf("expected closed after 2 failures, got %v", b.State())
	}

	b.Failure()
	if b.State() != StateOpen {
		t.Fatalf("expected open after 3 consecutive failures, got %v", b.State())
	}
}

func TestBreaker_OpenRejectsUntilCooldownElapses(t *testing.T) {
	t.Parallel()

	b, advance := newClockBreaker(t, 1, time.Minute)

	b.Failure() // 立即 open
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %v", b.State())
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
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", b.State())
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
	if b.State() != StateClosed {
		t.Fatalf("expected closed after probe success, got %v", b.State())
	}

	// 恢复 closed 后失败计数已复位：需要再次达到阈值才会熔断。
	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatalf("recovered call %d: expected allow", i+1)
		}
		b.Failure()
	}
	if b.State() != StateClosed {
		t.Fatalf("expected still closed after 2 post-recovery failures, got %v", b.State())
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
	if b.State() != StateOpen {
		t.Fatalf("expected re-open after failed probe, got %v", b.State())
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
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %v", b.State())
	}

	b.Failure()
	b.Failure()
	if b.State() != StateClosed {
		t.Fatalf("expected closed after streak reset + 2 failures, got %v", b.State())
	}

	b.Failure()
	if b.State() != StateOpen {
		t.Fatalf("expected open after 3 consecutive failures post-reset, got %v", b.State())
	}
}

func TestBreaker_GuardsInvalidConfig(t *testing.T) {
	t.Parallel()

	// maxFailures=0 按 1 处理；cooldown 为负按 0 处理（立即半开）。
	b := NewBreaker(0, -time.Second)

	b.Failure()
	if b.State() != StateOpen {
		t.Fatalf("expected open after single failure, got %v", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected immediate half-open with zero cooldown")
	}
}

func TestBreaker_OnTransitionCarriesFromAndTo(t *testing.T) {
	t.Parallel()

	var transitions [][2]State
	b, advance := newClockBreaker(t, 1, time.Minute)
	b.SetOnTransition(func(from, to State) {
		transitions = append(transitions, [2]State{from, to})
	})

	b.Failure() // closed → open
	if len(transitions) != 1 || transitions[0] != [2]State{StateClosed, StateOpen} {
		t.Fatalf("transitions = %v, want [[closed open]]", transitions)
	}

	advance(time.Minute)
	b.Allow() // open → half-open
	if len(transitions) != 2 || transitions[1] != [2]State{StateOpen, StateHalfOpen} {
		t.Fatalf("transitions = %v, want second [open half_open]", transitions)
	}

	b.Success() // half-open → closed
	if len(transitions) != 3 || transitions[2] != [2]State{StateHalfOpen, StateClosed} {
		t.Fatalf("transitions = %v, want third [half_open closed]", transitions)
	}
}

func TestBreaker_StringAndMetricValue(t *testing.T) {
	t.Parallel()

	if StateOpen.String() != "open" || StateHalfOpen.String() != "half_open" || StateClosed.String() != "closed" {
		t.Fatal("unexpected state names")
	}
	if State(42).String() != "unknown" {
		t.Fatalf("unknown state name = %q", State(42).String())
	}
	if StateClosed.MetricValue() != 0 || StateHalfOpen.MetricValue() != 1 || StateOpen.MetricValue() != 2 {
		t.Fatal("unexpected metric values")
	}
}

// ---------- 错误分类 / 退避 ----------

func TestStatusKind_Classification(t *testing.T) {
	t.Parallel()

	if StatusKind(http.StatusTooManyRequests) != KindTransient {
		t.Fatal("429 must be transient")
	}
	if StatusKind(http.StatusBadGateway) != KindTransient {
		t.Fatal("5xx must be transient")
	}
	if StatusKind(http.StatusNotFound) != KindBadRequest {
		t.Fatal("4xx (non-429) must be bad_request")
	}
}

func TestKindOf_NonKindErrorIsUnknown(t *testing.T) {
	t.Parallel()

	if KindOf(errors.New("plain")) != KindUnknown {
		t.Fatal("plain error must classify unknown")
	}
	if KindOf(NewError(KindCanceled, "x")) != KindCanceled {
		t.Fatal("KindError must carry its kind")
	}
}

func TestResultOf_Mappings(t *testing.T) {
	t.Parallel()

	cases := map[Kind]Result{
		KindTransient:  ResultTransient,
		KindBadRequest: ResultBadRequest,
		KindCanceled:   ResultCanceled,
		KindContract:   ResultContract,
	}
	for kind, want := range cases {
		if got := ResultOf(NewError(kind, "x")); got != want {
			t.Fatalf("kind %v → %q, want %q", kind, got, want)
		}
	}
	if ResultOf(errors.New("plain")) != ResultUnknown {
		t.Fatal("plain error must map unknown")
	}
}

func TestBackoffDelay_DoublingAndCaps(t *testing.T) {
	t.Parallel()

	r := RetryConfig{BaseDelay: 10 * time.Millisecond, MaxDelay: 40 * time.Millisecond}
	cases := []struct {
		failed int
		want   time.Duration
	}{
		{1, 10 * time.Millisecond},
		{2, 20 * time.Millisecond},
		{3, 40 * time.Millisecond},
		{10, 40 * time.Millisecond},
	}
	for _, c := range cases {
		if got := r.BackoffDelay(c.failed); got != c.want {
			t.Fatalf("backoff(%d) = %v, want %v", c.failed, got, c.want)
		}
	}
}

func TestBackoffDelay_CapsInsideDoublingLoop(t *testing.T) {
	t.Parallel()

	// 倍增过程中命中 maxDelay 即返回（走循环内封顶分支）。
	r := RetryConfig{BaseDelay: time.Millisecond, MaxDelay: 3 * time.Millisecond}
	if got := r.BackoffDelay(10); got != 3*time.Millisecond {
		t.Fatalf("backoffDelay(10) = %v, want 3ms cap", got)
	}
}

func TestBackoffDelay_FirstAttemptBaseOverMax(t *testing.T) {
	t.Parallel()

	// failedAttempts=1 时循环不执行，baseDelay 本身超过 maxDelay → 循环后封顶分支。
	r := RetryConfig{BaseDelay: 10 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	if got := r.BackoffDelay(1); got != 5*time.Millisecond {
		t.Fatalf("backoffDelay(1) = %v, want 5ms cap", got)
	}
}

func TestIsRetryable_OnlyTransient(t *testing.T) {
	t.Parallel()

	if !IsRetryable(NewError(KindTransient, "x")) {
		t.Fatal("transient must be retryable")
	}
	for _, k := range []Kind{KindBadRequest, KindCanceled, KindContract} {
		if IsRetryable(NewError(k, "x")) {
			t.Fatalf("kind %v must not be retryable", k)
		}
	}
	if IsRetryable(errors.New("plain")) {
		t.Fatal("non-KindError must not be retryable")
	}
}

func TestReportBreaker_RequestSideErrorsReportSuccess(t *testing.T) {
	t.Parallel()

	b, _ := newClockBreaker(t, 1, time.Minute)

	// 4xx 与上下文取消不算服务故障（上报成功，失败计数不涨）。
	ReportBreaker(b, NewError(KindBadRequest, "4xx"))
	ReportBreaker(b, NewError(KindCanceled, "canceled"))
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %v", b.State())
	}

	ReportBreaker(b, NewError(KindTransient, "5xx"))
	if b.State() != StateOpen {
		t.Fatalf("expected open after transient failure, got %v", b.State())
	}
}

func TestRetryConfig_Normalized(t *testing.T) {
	t.Parallel()

	r := RetryConfig{MaxAttempts: 0, BaseDelay: -time.Second, MaxDelay: -time.Second}.Normalize()
	if r.MaxAttempts != 1 || r.BaseDelay != 0 || r.MaxDelay != 0 {
		t.Fatalf("normalized = %+v", r)
	}
}

// ---------- Call 调用环 ----------

func TestCall_SuccessNoRetry(t *testing.T) {
	t.Parallel()

	var results []Result
	calls := 0
	got, err := Call(context.Background(), CallOptions{
		Retry:    DefaultRetryConfig(),
		OnResult: func(r Result) { results = append(results, r) },
	}, func(context.Context) (int, error) {
		calls++
		return 42, nil
	})
	if err != nil || got != 42 || calls != 1 {
		t.Fatalf("got=%d err=%v calls=%d, want 42/nil/1", got, err, calls)
	}
	if len(results) != 1 || results[0] != ResultSuccess {
		t.Fatalf("results = %v, want [success]", results)
	}
}

func TestCall_RetriesTransientWithBackoff(t *testing.T) {
	t.Parallel()

	attempts := 0
	var retries int
	got, err := Call(context.Background(), CallOptions{
		Retry:   RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond},
		Breaker: NewBreaker(DefaultMaxFailures, DefaultCooldown),
		Sleep:   func(time.Duration) {},
		OnRetry: func() { retries++ },
	}, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", NewError(KindTransient, "boom")
		}
		return "ok", nil
	})
	if err != nil || got != "ok" || attempts != 3 || retries != 2 {
		t.Fatalf("got=%q err=%v attempts=%d retries=%d", got, err, attempts, retries)
	}
}

func TestCall_NoRetryOnBadRequest(t *testing.T) {
	t.Parallel()

	attempts := 0
	_, err := Call(context.Background(), CallOptions{
		Retry: DefaultRetryConfig(),
		Sleep: func(time.Duration) {},
	}, func(context.Context) (int, error) {
		attempts++
		return 0, NewError(KindBadRequest, "nope")
	})
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (4xx not retried)", attempts)
	}
	if ResultOf(err) != ResultBadRequest {
		t.Fatalf("result = %q", ResultOf(err))
	}
}

func TestCall_CanceledBeforeStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	_, err := Call(ctx, CallOptions{Retry: DefaultRetryConfig()}, func(context.Context) (int, error) {
		calls++
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("err=%v calls=%d, want canceled/0", err, calls)
	}
}

func TestCall_CanceledDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := Call(ctx, CallOptions{
		Retry: RetryConfig{MaxAttempts: 5, BaseDelay: time.Hour},
		Sleep: func(time.Duration) { cancel() },
	}, func(context.Context) (int, error) {
		return 0, NewError(KindTransient, "503")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestCall_BreakerOpenFastFails(t *testing.T) {
	t.Parallel()

	b := NewBreaker(1, time.Minute)
	b.Failure() // open

	var results []Result
	calls := 0
	_, err := Call(context.Background(), CallOptions{
		Retry:    DefaultRetryConfig(),
		Breaker:  b,
		OnResult: func(r Result) { results = append(results, r) },
	}, func(context.Context) (int, error) {
		calls++
		return 0, nil
	})
	if !errors.Is(err, ErrBreakerOpen) || calls != 0 {
		t.Fatalf("err=%v calls=%d, want ErrBreakerOpen/0", err, calls)
	}
	if len(results) != 1 || results[0] != ResultBreakerOpen {
		t.Fatalf("results = %v, want [breaker_open]", results)
	}
}

func TestCall_UnknownErrorStillTripsBreaker(t *testing.T) {
	t.Parallel()

	b := NewBreaker(1, time.Minute)
	_, err := Call(context.Background(), CallOptions{
		Retry:   DefaultRetryConfig(),
		Breaker: b,
		Sleep:   func(time.Duration) {},
	}, func(context.Context) (int, error) {
		return 0, errors.New("plain")
	})
	if err == nil || ResultOf(err) != ResultUnknown {
		t.Fatalf("err=%v result=%q, want unknown", err, ResultOf(err))
	}
	if b.State() != StateOpen {
		t.Fatalf("expected breaker open after unknown error, got %v", b.State())
	}
}
