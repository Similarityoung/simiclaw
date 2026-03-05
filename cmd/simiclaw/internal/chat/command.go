package chat

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/channels/cli"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultBaseURL      = "http://127.0.0.1:8080"
	defaultConversation = "cli_default"
	defaultParticipant  = "local_user"
	defaultPollInterval = 50 * time.Millisecond
	defaultPollTimeout  = 60 * time.Second
	defaultReqTimeout   = 3 * time.Second
)

type Config struct {
	BaseURL      string
	Conversation string
	Participant  string
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
	return runREPL(ctx, os.Stdin, os.Stdout, client, cfg.Conversation, cfg.Participant, time.Now)
}

func parseConfig(args []string) (Config, error) {
	cfg := Config{
		BaseURL:      defaultBaseURL,
		Conversation: defaultConversation,
		Participant:  defaultParticipant,
	}
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to gateway config json")
	baseURL := fs.String("base-url", cfg.BaseURL, "gateway base url")
	conversation := fs.String("conversation", cfg.Conversation, "conversation id")
	participant := fs.String("participant", cfg.Participant, "participant id for dm conversation")
	apiKey := fs.String("api-key", "", "api key for Authorization header")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if strings.TrimSpace(*configPath) != "" {
		svcCfg, err := config.Load(strings.TrimSpace(*configPath))
		if err != nil {
			return Config{}, fmt.Errorf("load chat config: %w", err)
		}
		if !setFlags["base-url"] {
			cfg.BaseURL = baseURLFromListenAddr(svcCfg.ListenAddr)
		}
		if !setFlags["api-key"] {
			cfg.APIKey = strings.TrimSpace(svcCfg.APIKey)
		}
	}

	if setFlags["base-url"] {
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(*baseURL), "/")
	}
	cfg.Conversation = strings.TrimSpace(*conversation)
	cfg.Participant = strings.TrimSpace(*participant)
	if setFlags["api-key"] {
		cfg.APIKey = strings.TrimSpace(*apiKey)
	}

	if cfg.BaseURL == "" {
		return Config{}, errors.New("base-url is required")
	}
	if cfg.Conversation == "" {
		return Config{}, errors.New("conversation is required")
	}
	if cfg.Participant == "" {
		return Config{}, errors.New("participant is required")
	}
	return cfg, nil
}

func runREPL(ctx context.Context, in io.Reader, out io.Writer, client ChatClient, conversationID, participantID string, now func() time.Time) error {
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

		req := cli.BuildIngestRequest(conversationID, participantID, seq, text)
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

		if rec.DeliveryStatus == model.DeliveryStatusFailed && rec.Error != nil {
			if _, err := fmt.Fprintf(out, "error> %s: %s\n", rec.Error.Code, rec.Error.Message); err != nil {
				return err
			}
		}
	}
}

func baseURLFromListenAddr(listenAddr string) string {
	raw := strings.TrimSpace(listenAddr)
	if raw == "" {
		return defaultBaseURL
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return strings.TrimRight(raw, "/")
	}

	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		if strings.HasPrefix(raw, ":") {
			host = "127.0.0.1"
			port = strings.TrimPrefix(raw, ":")
		} else {
			return defaultBaseURL
		}
	}

	if strings.TrimSpace(port) == "" {
		return defaultBaseURL
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
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
