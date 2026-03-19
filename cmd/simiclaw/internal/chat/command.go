package chat

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	sharedclient "github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/ui/messages"
	"github.com/spf13/cobra"
)

const (
	fixedParticipantID = "local_user"
)

type Options struct {
	Conversation string
	SessionKey   string
	NewSession   bool
	NoStream     bool
	HistoryLimit int
}

func NewCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	opts := Options{}
	cmd := &cobra.Command{
		Use:   "chat",
		Short: messages.Command.ChatShort,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !common.IsInteractive(streams) {
				return common.WrapExit(1, errors.New(messages.InteractiveTerminalRequired("chat")))
			}
			runtimeOpts, err := common.ResolveRuntimeOptions(*globals, streams.Out)
			if err != nil {
				return common.WrapExit(2, err)
			}
			return runTUI(cmd.Context(), streams, sharedclient.New(runtimeOpts.BaseURL, runtimeOpts.APIKey, runtimeOpts.Timeout), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Conversation, "conversation", "", messages.Flag.ConversationID)
	cmd.Flags().StringVar(&opts.SessionKey, "session-key", "", messages.Flag.SessionKey)
	cmd.Flags().BoolVar(&opts.NewSession, "new", false, messages.Flag.NewSession)
	cmd.Flags().BoolVar(&opts.NoStream, "no-stream", false, messages.Flag.NoStream)
	cmd.Flags().IntVar(&opts.HistoryLimit, "history-limit", 50, messages.Flag.HistoryLimit)
	return cmd
}

func runTUI(ctx context.Context, streams common.IOStreams, cli *sharedclient.Client, opts Options) error {
	model := newModel(ctx, streams, cli, opts)
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(*modelState); ok && m.finalErr != nil {
		return m.finalErr
	}
	return nil
}
