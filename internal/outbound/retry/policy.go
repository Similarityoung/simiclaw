package retry

import "time"

const (
	defaultBaseDelay = 5 * time.Second
	defaultMaxDelay  = 5 * time.Minute
	defaultAttempts  = 5
)

type Decision struct {
	NextAttemptAt time.Time
	Dead          bool
}

type Policy interface {
	Next(attemptCount int, now time.Time) Decision
}

type ExponentialPolicy struct {
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxAttempts int
}

func DefaultPolicy() ExponentialPolicy {
	return ExponentialPolicy{
		BaseDelay:   defaultBaseDelay,
		MaxDelay:    defaultMaxDelay,
		MaxAttempts: defaultAttempts,
	}
}

func (p ExponentialPolicy) Next(attemptCount int, now time.Time) Decision {
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaultBaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaultMaxDelay
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaultAttempts
	}
	if attemptCount >= p.MaxAttempts {
		return Decision{NextAttemptAt: now, Dead: true}
	}

	delay := p.BaseDelay
	for i := 0; i < attemptCount; i++ {
		if delay >= p.MaxDelay {
			delay = p.MaxDelay
			break
		}
		delay *= 2
		if delay > p.MaxDelay {
			delay = p.MaxDelay
			break
		}
	}
	return Decision{
		NextAttemptAt: now.Add(delay),
		Dead:          false,
	}
}
