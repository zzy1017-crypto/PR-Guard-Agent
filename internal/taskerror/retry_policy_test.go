package taskerror

import (
	"testing"
	"time"
)

func TestRetryPolicyExponentialBackoffWithoutJitter(t *testing.T) {
	policy := RetryPolicy{BaseDelay: 2 * time.Second, MaxDelay: 30 * time.Second}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 2 * time.Second},
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
		{attempt: 10, want: 30 * time.Second},
	}
	for _, test := range tests {
		if got := policy.NextDelay(test.attempt); got != test.want {
			t.Errorf("NextDelay(%d) = %s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestRetryPolicyZeroJitterIsStable(t *testing.T) {
	policy := RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Minute, JitterPercent: 0}
	first := policy.NextDelay(4)
	for index := 0; index < 100; index++ {
		if got := policy.NextDelay(4); got != first {
			t.Fatalf("NextDelay() changed from %s to %s", first, got)
		}
	}
}
