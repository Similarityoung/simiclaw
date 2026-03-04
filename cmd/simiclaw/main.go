package main

import (
	"fmt"
	"os"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/chat"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/initcmd"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/version"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func main() {
	if err := common.SetupLogger("info"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer common.SyncLogger()

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
