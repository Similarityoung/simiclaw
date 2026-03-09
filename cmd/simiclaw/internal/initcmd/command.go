package initcmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/ui/messages"
	"github.com/similarityyoung/simiclaw/pkg/config"
)

type Options struct {
	Workspace       string
	ForceNewRuntime bool
}

func Run(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	workspace := fs.String("workspace", ".", messages.Flag.WorkspacePath)
	forceNewRuntime := fs.Bool("force-new-runtime", false, messages.Flag.ForceNewRuntime)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return run(Options{Workspace: *workspace, ForceNewRuntime: *forceNewRuntime}, common.IOStreams{})
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
	if err := scaffoldWorkspaceFiles(opts.Workspace); err != nil {
		return err
	}
	out := streams.Out
	if out == nil {
		out = os.Stdout
	}
	_, err := fmt.Fprint(out, messages.WorkspaceInitialized(opts.Workspace))
	return err
}
