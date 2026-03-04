package common

import (
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func SetupLogger(level string) error {
	return logging.Init(level)
}

func SyncLogger() {
	logging.Sync()
}
