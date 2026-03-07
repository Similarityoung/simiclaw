package initcmd

import (
	"flag"
	"fmt"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace path")
	forceNewRuntime := fs.Bool("force-new-runtime", false, "remove legacy runtime traces and create a fresh SQLite runtime")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := store.InitWorkspace(*workspace, *forceNewRuntime, config.Default().DBBusyTimeout.Duration); err != nil {
		return err
	}
	fmt.Printf("workspace initialized at %s\n", *workspace)
	return nil
}
