package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/chat"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/initcmd"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/inspect"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/version"
)

func NewCommand(streams common.IOStreams) *cobra.Command {
	globals := &common.RuntimeFlagValues{}
	cmd := &cobra.Command{
		Use:           "simiclaw",
		Short:         "SimiClaw CLI v2",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)
	cmd.PersistentFlags().StringVar(&globals.BaseURL, "base-url", "", "gateway base URL")
	cmd.PersistentFlags().StringVar(&globals.APIKey, "api-key", "", "API key for Authorization header")
	cmd.PersistentFlags().DurationVar(&globals.Timeout, "timeout", 0, "request timeout")
	cmd.PersistentFlags().StringVar(&globals.Output, "output", "", "output format: table or json")
	cmd.PersistentFlags().BoolVar(&globals.NoColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVar(&globals.Verbose, "verbose", false, "verbose output")

	cmd.AddCommand(initcmd.NewCommand(streams))
	cmd.AddCommand(gateway.NewCommand())
	cmd.AddCommand(chat.NewCommand(streams, globals))
	cmd.AddCommand(inspect.NewCommand(streams, globals))
	cmd.AddCommand(version.NewCommand(streams.Out))
	cmd.AddCommand(newCompletionCommand(streams))
	return cmd
}

func Execute(args []string, streams common.IOStreams) error {
	cmd := NewCommand(streams)
	if len(args) == 0 {
		if common.IsInteractive(streams) {
			args = []string{"chat"}
		} else {
			args = []string{"--help"}
		}
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func newCompletionCommand(streams common.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "生成 shell completion",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(streams.Out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(streams.Out)
			case "fish":
				return cmd.Root().GenFishCompletion(streams.Out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(streams.Out)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
	return cmd
}
