package store

import "time"

func DefaultBusyTimeout() time.Duration {
	return 5 * time.Second
}
