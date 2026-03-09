package messages

import "fmt"

type CommandText struct {
	RootShort       string
	ChatShort       string
	GatewayShort    string
	InitShort       string
	InspectShort    string
	InspectHealth   string
	InspectSessions string
	InspectEvents   string
	InspectRuns     string
	InspectTrace    string
	VersionShort    string
	CompletionShort string
}

var Command = CommandText{
	RootShort:       "SimiClaw CLI v2",
	ChatShort:       "Start the interactive chat TUI",
	GatewayShort:    "Start the HTTP server",
	InitShort:       "Initialize a workspace",
	InspectShort:    "Inspect service state and data",
	InspectHealth:   "Check healthz and readyz",
	InspectSessions: "List sessions",
	InspectEvents:   "List events",
	InspectRuns:     "List runs",
	InspectTrace:    "Show a run trace",
	VersionShort:    "Print the version",
	CompletionShort: "Generate shell completion",
}

type FlagText struct {
	BaseURL              string
	APIKey               string
	RequestTimeout       string
	OutputFormat         string
	NoColor              string
	Verbose              string
	ConversationID       string
	SessionKey           string
	NewSession           string
	NoStream             string
	HistoryLimit         string
	ConfigJSON           string
	WorkspaceOverride    string
	ListenOverride       string
	WorkspacePath        string
	ForceNewRuntime      string
	ItemsToReturn        string
	PaginationCursor     string
	FilterBySessionKey   string
	FilterByConversation string
	FilterByStatus       string
}

var Flag = FlagText{
	BaseURL:              "gateway base URL",
	APIKey:               "API key for Authorization header",
	RequestTimeout:       "request timeout",
	OutputFormat:         "output format: table or json",
	NoColor:              "disable color output",
	Verbose:              "verbose output",
	ConversationID:       "conversation id",
	SessionKey:           "session key",
	NewSession:           "create a new session",
	NoStream:             "disable streaming chat",
	HistoryLimit:         "history items to load",
	ConfigJSON:           "config json file",
	WorkspaceOverride:    "workspace override",
	ListenOverride:       "listen address override",
	WorkspacePath:        "workspace path",
	ForceNewRuntime:      "remove legacy runtime traces and create a fresh SQLite runtime",
	ItemsToReturn:        "items to return",
	PaginationCursor:     "pagination cursor",
	FilterBySessionKey:   "filter by session_key",
	FilterByConversation: "filter by conversation_id",
	FilterByStatus:       "filter by status",
}

func UnsupportedShell(shell string) string {
	return fmt.Sprintf("unsupported shell %q", shell)
}

func InteractiveTerminalRequired(command string) string {
	return fmt.Sprintf("%s requires an interactive terminal", command)
}

func WorkspaceInitialized(path string) string {
	return fmt.Sprintf("workspace initialized at %s\n", path)
}
