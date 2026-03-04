package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/chat"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/initcmd"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/version"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

const bootstrapLogLevelEnv = "SIMICLAW_LOG_LEVEL"

func main() {
	if err := common.SetupLogger(resolveBootstrapLogLevel()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := common.SyncLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "logger sync failed: %v\n", err)
		}
	}()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	var err error
	switch cmd {
	case "init":
		err = initcmd.Run(os.Args[2:])
	case "serve", "gateway":
		err = gateway.Run(os.Args[2:])
	case "chat":
		err = chat.Run(os.Args[2:])
	case "version":
		version.Run()
		return
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		logging.L("cmd").Error(cmd+" failed", logging.Error(err))
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: simiclaw <init|serve|gateway|chat|version> [flags]")
}

func resolveBootstrapLogLevel() string {
	if level := strings.TrimSpace(os.Getenv(bootstrapLogLevelEnv)); level != "" {
		return level
	}
	return config.Default().LogLevel
}
