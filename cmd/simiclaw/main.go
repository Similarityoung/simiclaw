package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/root"
	"github.com/similarityyoung/simiclaw/pkg/config"
)

const bootstrapLogLevelEnv = "SIMICLAW_LOG_LEVEL"

func main() {
	os.Exit(run(os.Args[1:], common.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}))
}

func run(args []string, streams common.IOStreams) int {
	if err := common.SetupLogger(resolveBootstrapLogLevel()); err != nil {
		fmt.Fprintln(streams.ErrOut, err)
		return 1
	}
	defer func() {
		if err := common.SyncLogger(); err != nil {
			fmt.Fprintf(streams.ErrOut, "logger sync failed: %v\n", err)
		}
	}()

	err := root.Execute(args, streams)
	if err == nil {
		return 0
	}
	if errors.Is(err, pflag.ErrHelp) {
		return 0
	}
	fmt.Fprintln(streams.ErrOut, err)
	if common.AsExitError(err, nil) {
		return common.ExitCode(err)
	}
	if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "accepts ") {
		return 2
	}
	return 1
}

func resolveBootstrapLogLevel() string {
	if level := strings.TrimSpace(os.Getenv(bootstrapLogLevelEnv)); level != "" {
		return level
	}
	return config.Default().LogLevel
}
