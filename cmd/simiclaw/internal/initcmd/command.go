package initcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/messages"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	workspacepkg "github.com/similarityyoung/simiclaw/internal/workspace"
)

type Options struct {
	Workspace       string
	ForceNewRuntime bool
}

func NewCommand(streams common.IOStreams) *cobra.Command {
	opts := Options{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: messages.Command.InitShort,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(opts, streams)
		},
	}
	cmd.Flags().StringVar(&opts.Workspace, "workspace", ".", messages.Flag.WorkspacePath)
	cmd.Flags().BoolVar(&opts.ForceNewRuntime, "force-new-runtime", false, messages.Flag.ForceNewRuntime)
	return cmd
}

func run(opts Options, streams common.IOStreams) error {
	if opts.Workspace == "" {
		opts.Workspace = "."
	}
	if err := store.InitWorkspace(opts.Workspace, opts.ForceNewRuntime, config.Default().DBBusyTimeout.Duration); err != nil {
		return err
	}
	if err := workspacepkg.ScaffoldFiles(opts.Workspace); err != nil {
		return err
	}
	out := streams.Out
	if out == nil {
		out = os.Stdout
	}
	_, err := fmt.Fprint(out, messages.WorkspaceInitialized(opts.Workspace))
	return err
}
