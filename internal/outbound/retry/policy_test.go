package retry

import (
	"testing"
	"time"
)

func TestDefaultPolicyUsesBoundedExponentialBackoff(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	policy := DefaultPolicy()

	first := policy.Next(0, now)
	if first.Dead || !first.NextAttemptAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("unexpected first retry decision: %+v", first)
	}
	third := policy.Next(2, now)
	if third.Dead || !third.NextAttemptAt.Equal(now.Add(20*time.Second)) {
		t.Fatalf("unexpected third retry decision: %+v", third)
	}
	dead := policy.Next(5, now)
	if !dead.Dead || !dead.NextAttemptAt.Equal(now) {
		t.Fatalf("unexpected dead-letter decision: %+v", dead)
	}
}
