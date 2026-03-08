package common

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	DefaultBaseURL = "http://127.0.0.1:8080"
	DefaultTimeout = 10 * time.Second

	EnvBaseURL = "SIMICLAW_BASE_URL"
	EnvAPIKey  = "SIMICLAW_API_KEY"
	EnvTimeout = "SIMICLAW_TIMEOUT"
	EnvOutput  = "SIMICLAW_OUTPUT"
	EnvNoColor = "SIMICLAW_NO_COLOR"
	EnvVerbose = "SIMICLAW_VERBOSE"
)

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

type RuntimeFlagValues struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Output  string
	NoColor bool
	Verbose bool
}

type RuntimeOptions struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Output  string
	NoColor bool
	Verbose bool
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func WrapExit(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if ok := AsExitError(err, &exitErr); ok && exitErr != nil && exitErr.Code > 0 {
		return exitErr.Code
	}
	return 1
}

func AsExitError(err error, target **ExitError) bool {
	exitErr, ok := err.(*ExitError)
	if ok && target != nil {
		*target = exitErr
	}
	return ok
}

func ResolveRuntimeOptions(flags RuntimeFlagValues, out io.Writer) (RuntimeOptions, error) {
	timeout := flags.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
		if raw := strings.TrimSpace(os.Getenv(EnvTimeout)); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				return RuntimeOptions{}, fmt.Errorf("invalid %s: %w", EnvTimeout, err)
			}
			timeout = parsed
		}
	}
	output := normalizeString(flags.Output)
	if output == "" {
		output = normalizeString(os.Getenv(EnvOutput))
	}
	if output == "" {
		if IsTerminalWriter(out) {
			output = "table"
		} else {
			output = "json"
		}
	}
	if output != "table" && output != "json" {
		return RuntimeOptions{}, fmt.Errorf("invalid output %q", output)
	}
	baseURL := normalizeString(flags.BaseURL)
	if baseURL == "" {
		baseURL = normalizeString(os.Getenv(EnvBaseURL))
		if baseURL == "" {
			baseURL = DefaultBaseURL
		}
	}
	apiKey := strings.TrimSpace(flags.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(EnvAPIKey))
	}
	noColor := flags.NoColor || parseBoolEnv(EnvNoColor) || parseBoolEnv("NO_COLOR")
	verbose := flags.Verbose || parseBoolEnv(EnvVerbose)
	return RuntimeOptions{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Timeout: timeout,
		Output:  output,
		NoColor: noColor,
		Verbose: verbose,
	}, nil
}

func IsTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func IsTerminalReader(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func IsInteractive(streams IOStreams) bool {
	return IsTerminalReader(streams.In) && IsTerminalWriter(streams.Out)
}

func normalizeString(v string) string {
	return strings.TrimSpace(v)
}

func parseBoolEnv(key string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
