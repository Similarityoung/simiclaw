package initcmd

import (
	"flag"
	"fmt"

	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", "workspace path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := store.InitWorkspace(*workspace); err != nil {
		return err
	}
	fmt.Printf("workspace initialized at %s\n", *workspace)
	return nil
}
