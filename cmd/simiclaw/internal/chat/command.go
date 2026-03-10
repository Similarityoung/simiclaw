package chat

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	sharedclient "github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/internal/ui/messages"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultBaseURL      = "http://127.0.0.1:8080"
	defaultConversation = "cli_default"
	fixedParticipantID  = "local_user"
)

type Options struct {
	Conversation string
	SessionKey   string
	NewSession   bool
	NoStream     bool
	HistoryLimit int
}

type ChatClient interface {
	SendAndWait(ctx context.Context, req api.IngestRequest) (api.EventRecord, error)
	SendStream(ctx context.Context, req api.IngestRequest, handler StreamEventHandler) (api.EventRecord, error)
	PollEvent(ctx context.Context, eventID string) (api.EventRecord, error)
}

type replInput struct {
	text string
	err  error
	eof  bool
}

func Run(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	opts := Options{}
	baseURL := fs.String("base-url", common.DefaultBaseURL, messages.Flag.BaseURL)
	apiKey := fs.String("api-key", "", messages.Flag.APIKey)
	fs.StringVar(&opts.Conversation, "conversation", "", messages.Flag.ConversationID)
	fs.StringVar(&opts.SessionKey, "session-key", "", messages.Flag.SessionKey)
	fs.BoolVar(&opts.NewSession, "new", false, messages.Flag.NewSession)
	fs.BoolVar(&opts.NoStream, "no-stream", false, messages.Flag.NoStream)
	fs.IntVar(&opts.HistoryLimit, "history-limit", 50, messages.Flag.HistoryLimit)
	if err := fs.Parse(args); err != nil {
		return err
	}
	runtimeOpts, err := common.ResolveRuntimeOptions(common.RuntimeFlagValues{BaseURL: *baseURL, APIKey: *apiKey}, os.Stdout)
	if err != nil {
		return err
	}
	return runTUI(common.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}, sharedclient.New(runtimeOpts.BaseURL, runtimeOpts.APIKey, runtimeOpts.Timeout), opts)
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
			return runTUI(streams, sharedclient.New(runtimeOpts.BaseURL, runtimeOpts.APIKey, runtimeOpts.Timeout), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Conversation, "conversation", "", messages.Flag.ConversationID)
	cmd.Flags().StringVar(&opts.SessionKey, "session-key", "", messages.Flag.SessionKey)
	cmd.Flags().BoolVar(&opts.NewSession, "new", false, messages.Flag.NewSession)
	cmd.Flags().BoolVar(&opts.NoStream, "no-stream", false, messages.Flag.NoStream)
	cmd.Flags().IntVar(&opts.HistoryLimit, "history-limit", 50, messages.Flag.HistoryLimit)
	return cmd
}

func runTUI(streams common.IOStreams, cli *sharedclient.Client, opts Options) error {
	model := newModel(streams, cli, opts)
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

func runREPL(ctx context.Context, in io.Reader, out io.Writer, client ChatClient, conversationID string, useStream bool, now func() time.Time) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	inputCh := startScanner(ctx, scanner)
	seq := now().UnixMilli()
	if conversationID == "" {
		conversationID = defaultConversation
	}

	for {
		if _, err := fmt.Fprint(out, messages.Chat.REPLPrompt); err != nil {
			return err
		}

		var (
			input replInput
			ok    bool
		)
		select {
		case <-ctx.Done():
			return nil
		case input, ok = <-inputCh:
			if !ok {
				return nil
			}
		}
		if input.err != nil {
			if ctx.Err() == nil {
				return input.err
			}
			return nil
		}
		if input.eof {
			return nil
		}

		text := strings.TrimSpace(input.text)
		if text == "" {
			continue
		}
		if text == "/quit" || text == "/exit" {
			return nil
		}

		req := cli.BuildIngestRequest(conversationID, fixedParticipantID, seq, text)
		seq++
		renderer := newStreamRenderer(out)
		rec, err := sendOneTurn(ctx, client, req, useStream, renderer)
		if err != nil {
			renderer.Abort()
			if _, werr := fmt.Fprint(out, messages.Chat.REPLError(formatError(err))); werr != nil {
				return werr
			}
			continue
		}
		if err := renderer.Finish(rec); err != nil {
			return err
		}
		if rec.Status == model.EventStatusFailed {
			if rec.Error != nil {
				if _, werr := fmt.Fprint(out, messages.Chat.REPLErrorCode(rec.Error.Code, rec.Error.Message)); werr != nil {
					return werr
				}
			} else {
				if _, werr := fmt.Fprintln(out, messages.Chat.REPLEventFailed); werr != nil {
					return werr
				}
			}
			continue
		}
		if rec.OutboxStatus == model.OutboxStatusDead && rec.Error != nil {
			if _, err := fmt.Fprint(out, messages.Chat.REPLErrorCode(rec.Error.Code, rec.Error.Message)); err != nil {
				return err
			}
		}
	}
}

func startScanner(ctx context.Context, scanner *bufio.Scanner) <-chan replInput {
	out := make(chan replInput, 1)
	go func() {
		defer close(out)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case out <- replInput{text: line}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case out <- replInput{err: err}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case out <- replInput{eof: true}:
		case <-ctx.Done():
		}
	}()
	return out
}

func formatError(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Error()
	}
	return err.Error()
}

func sendOneTurn(ctx context.Context, client ChatClient, req api.IngestRequest, useStream bool, renderer *streamRenderer) (api.EventRecord, error) {
	if !useStream {
		return client.SendAndWait(ctx, req)
	}
	rec, err := client.SendStream(ctx, req, renderer)
	if err == nil {
		return rec, nil
	}
	var recoverable *StreamRecoverableError
	switch {
	case errors.Is(err, ErrStreamUnsupported):
		return client.SendAndWait(ctx, req)
	case errors.As(err, &recoverable) && recoverable.EventID != "":
		return client.PollEvent(ctx, recoverable.EventID)
	default:
		return api.EventRecord{}, err
	}
}
