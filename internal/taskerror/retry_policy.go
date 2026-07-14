package taskerror

import (
	"math"
	"math/rand/v2"
	"time"
)

type RetryPolicy struct {
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	JitterPercent int
}

func (p RetryPolicy) NextDelay(attemptCount int) time.Duration {
	if p.BaseDelay <= 0 || p.MaxDelay <= 0 {
		return 0
	}
	if attemptCount < 1 {
		attemptCount = 1
	}

	delay := p.BaseDelay
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	for attempt := 1; attempt < attemptCount && delay < p.MaxDelay; attempt++ {
		if delay > p.MaxDelay/2 {
			delay = p.MaxDelay
			break
		}
		delay *= 2
	}

	percent := p.JitterPercent
	if percent < 0 {
		percent = 0
	}
	if percent > 50 {
		percent = 50
	}
	if percent == 0 || delay == 0 {
		return delay
	}

	jitterRange := durationPercent(delay, percent)
	if jitterRange <= 0 {
		return delay
	}
	var offset time.Duration
	if jitterRange <= time.Duration((math.MaxInt64-1)/2) {
		offset = time.Duration(rand.Int64N(int64(jitterRange)*2+1)) - jitterRange
	} else {
		offset = time.Duration((rand.Float64()*2 - 1) * float64(jitterRange))
	}
	if offset < 0 && -offset > delay {
		return 0
	}
	if offset > 0 && delay > p.MaxDelay-offset {
		return p.MaxDelay
	}
	delay += offset
	if delay < 0 {
		return 0
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

func durationPercent(value time.Duration, percent int) time.Duration {
	return value/100*time.Duration(percent) + value%100*time.Duration(percent)/100
}
