package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/chat"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/initcmd"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/version"
)

func main() {
	common.SetupDefaultLogger()

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
		slog.Error(cmd+" failed", "error", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: simiclaw <init|serve|gateway|chat|version> [flags]")
}
