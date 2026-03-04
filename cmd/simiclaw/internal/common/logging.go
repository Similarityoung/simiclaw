package common

import (
	"log/slog"
	"os"
)

func SetupDefaultLogger() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
}
