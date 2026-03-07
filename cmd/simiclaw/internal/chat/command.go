package chat

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultBaseURL      = "http://127.0.0.1:8080"
	defaultConversation = "cli_default"
	fixedParticipantID  = "local_user"
	defaultPollInterval = 50 * time.Millisecond
	defaultPollTimeout  = 60 * time.Second
	defaultReqTimeout   = 3 * time.Second
)

type Config struct {
	BaseURL      string
	Conversation string
	APIKey       string
}

type ChatClient interface {
	SendAndWait(ctx context.Context, req model.IngestRequest) (model.EventRecord, error)
}

type replInput struct {
	text string
	err  error
	eof  bool
}

func Run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := NewHTTPClient(cfg.BaseURL, cfg.APIKey, defaultReqTimeout, defaultPollInterval, defaultPollTimeout)
	return runREPL(ctx, os.Stdin, os.Stdout, client, cfg.Conversation, time.Now)
}

func parseConfig(args []string) (Config, error) {
	cfg := Config{
		BaseURL:      defaultBaseURL,
		Conversation: defaultConversation,
	}
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	baseURL := fs.String("base-url", cfg.BaseURL, "gateway base url")
	conversation := fs.String("conversation", cfg.Conversation, "conversation id")
	apiKey := fs.String("api-key", "", "api key for Authorization header")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(*baseURL), "/")
	cfg.Conversation = strings.TrimSpace(*conversation)
	cfg.APIKey = strings.TrimSpace(*apiKey)

	if cfg.BaseURL == "" {
		return Config{}, errors.New("base-url is required")
	}
	if cfg.Conversation == "" {
		return Config{}, errors.New("conversation is required")
	}
	return cfg, nil
}

func runREPL(ctx context.Context, in io.Reader, out io.Writer, client ChatClient, conversationID string, now func() time.Time) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	inputCh := startScanner(ctx, scanner)
	seq := now().UnixMilli()

	for {
		if _, err := fmt.Fprint(out, "you> "); err != nil {
			return err
		}

		var (
			in replInput
			ok bool
		)
		select {
		case <-ctx.Done():
			return nil
		case in, ok = <-inputCh:
			if !ok {
				return nil
			}
		}
		if in.err != nil {
			if ctx.Err() == nil {
				return in.err
			}
			return nil
		}
		if in.eof {
			return nil
		}

		text := strings.TrimSpace(in.text)
		if text == "" {
			continue
		}
		if text == "/quit" || text == "/exit" {
			return nil
		}

		req := cli.BuildIngestRequest(conversationID, fixedParticipantID, seq, text)
		seq++
		rec, err := client.SendAndWait(ctx, req)
		if err != nil {
			if _, werr := fmt.Fprintf(out, "error> %s\n", formatError(err)); werr != nil {
				return werr
			}
			continue
		}
		if rec.Status == model.EventStatusFailed {
			if rec.Error != nil {
				if _, werr := fmt.Fprintf(out, "error> %s: %s\n", rec.Error.Code, rec.Error.Message); werr != nil {
					return werr
				}
			} else {
				if _, werr := fmt.Fprintln(out, "error> event failed"); werr != nil {
					return werr
				}
			}
			continue
		}

		reply := rec.AssistantReply
		if reply == "" {
			reply = "(no reply)"
		}
		if _, err := fmt.Fprintf(out, "bot> %s\n", reply); err != nil {
			return err
		}

		if rec.OutboxStatus == model.OutboxStatusDead && rec.Error != nil {
			if _, err := fmt.Fprintf(out, "error> %s: %s\n", rec.Error.Code, rec.Error.Message); err != nil {
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
