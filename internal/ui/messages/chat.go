package messages

import "fmt"

type ChatText struct {
	InputPlaceholder              string
	StatusLoadingSessions         string
	StatusNewSessionPending       string
	StatusCheckingConversation    string
	StatusCheckConversationFailed string
	StatusLoadingHistory          string
	StatusLoadFailed              string
	StatusNoSessions              string
	StatusMessageSentWait         string
	StatusSendFailed              string
	StatusRefreshingSessions      string
	StatusLoadingSessionList      string
	StatusRequestAccepted         string
	StatusProcessing              string
	StatusReasoning               string
	StatusStreamingReply          string
	StatusRunFailed               string
	StatusDone                    string
	SelectorTitle                 string
	SelectorHelp                  string
	SelectorLoading               string
	SelectorEmpty                 string
	NamingViewTemplate            string
	HeaderTemplate                string
	Help                          string
	StatusErrorPrefix             string
	NoMessages                    string
	ConversationMissing           string
	SessionPending                string
	REPLPrompt                    string
	REPLErrorTemplate             string
	REPLErrorCodeTemplate         string
	REPLEventFailed               string
	StreamStatusTemplate          string
	StreamThinkingTemplate        string
	StreamToolStartTemplate       string
	StreamToolResultTemplate      string
	StreamNoReply                 string
	StreamBotPrompt               string
	StreamBotLineTemplate         string
	StreamBotRewriteTemplate      string
	MessagePrefixUser             string
	MessagePrefixAssistant        string
}

var Chat = ChatText{
	InputPlaceholder:              "Type a message. Enter sends, Ctrl+J adds a newline",
	StatusLoadingSessions:         "Loading sessions…",
	StatusNewSessionPending:       "New session. The first message will create the remote session",
	StatusCheckingConversation:    "Checking conversation_id…",
	StatusCheckConversationFailed: "Failed to check conversation_id",
	StatusLoadingHistory:          "Loading session history…",
	StatusLoadFailed:              "Failed to load sessions",
	StatusNoSessions:              "No sessions yet. Press Ctrl+N to create one",
	StatusMessageSentWait:         "Message sent. Waiting for reply…",
	StatusSendFailed:              "Send failed",
	StatusRefreshingSessions:      "Refreshing sessions…",
	StatusLoadingSessionList:      "Loading session list…",
	StatusRequestAccepted:         "Request accepted",
	StatusProcessing:              "Processing…",
	StatusReasoning:               "Reasoning…",
	StatusStreamingReply:          "Streaming reply…",
	StatusRunFailed:               "Run failed",
	StatusDone:                    "Done",
	SelectorTitle:                 "SimiClaw Chat\n",
	SelectorHelp:                  "Select a session (↑/↓ or j/k, Enter opens, Ctrl+N creates, r refreshes, Ctrl+C quits)\n\n",
	SelectorLoading:               "Loading…\n",
	SelectorEmpty:                 "No sessions\n",
	NamingViewTemplate:            "Create a new session\n\nEnter conversation_id (leave blank to use the default)\n\n%s\n\nEnter confirms, Esc goes back, Ctrl+C quits\n\n%s",
	HeaderTemplate:                "SimiClaw Chat  ·  conversation=%s  ·  session=%s",
	Help:                          "Enter sends  ·  Ctrl+J newline  ·  Ctrl+R resend  ·  Ctrl+O sessions  ·  Ctrl+N new session  ·  Ctrl+C quit",
	StatusErrorPrefix:             "error=",
	NoMessages:                    "No messages yet. Type the first one to start chatting.",
	ConversationMissing:           "(not created)",
	SessionPending:                "(created after the first message)",
	REPLPrompt:                    "you> ",
	REPLErrorTemplate:             "error> %s\n",
	REPLErrorCodeTemplate:         "error> %s: %s\n",
	REPLEventFailed:               "error> event failed",
	StreamStatusTemplate:          "status> %s\n",
	StreamThinkingTemplate:        "think> %s\n",
	StreamToolStartTemplate:       "tool> [%s] %s %s\n",
	StreamToolResultTemplate:      "tool< [%s] %s %s\n",
	StreamNoReply:                 "(no reply)",
	StreamBotPrompt:               "bot> ",
	StreamBotLineTemplate:         "bot> %s\n",
	StreamBotRewriteTemplate:      "\r\033[2Kbot> %s\n",
	MessagePrefixUser:             "you",
	MessagePrefixAssistant:        "bot",
}

func (c ChatText) LoadedSessions(count int) string {
	return fmt.Sprintf("Loaded %d sessions", count)
}

func (c ChatText) OpenedSession(conversation string) string {
	return fmt.Sprintf("Opened session %s", conversation)
}

func (c ChatText) NewSession(conversation string) string {
	return fmt.Sprintf("New session %s", conversation)
}

func (c ChatText) ConversationExists(conversation string) string {
	return fmt.Sprintf("conversation_id %s already exists. Choose another one", conversation)
}

func (c ChatText) RunningTool(name string) string {
	return fmt.Sprintf("Running tool: %s", name)
}

func (c ChatText) ToolFailed(name string) string {
	return fmt.Sprintf("Tool failed: %s", name)
}

func (c ChatText) ToolCompleted(name string) string {
	return fmt.Sprintf("Tool completed: %s", name)
}

func (c ChatText) SelectorItem(cursor, conversation string, messageCount int, lastModel, lastActivity string) string {
	return fmt.Sprintf("%s%s  ·  %d messages  ·  %s  ·  %s\n", cursor, conversation, messageCount, lastModel, lastActivity)
}

func (c ChatText) NamingView(inputView, status string) string {
	return fmt.Sprintf(c.NamingViewTemplate, inputView, status)
}

func (c ChatText) Header(conversation, session string) string {
	return fmt.Sprintf(c.HeaderTemplate, conversation, session)
}

func (c ChatText) REPLError(message string) string {
	return fmt.Sprintf(c.REPLErrorTemplate, message)
}

func (c ChatText) REPLErrorCode(code, message string) string {
	return fmt.Sprintf(c.REPLErrorCodeTemplate, code, message)
}

func (c ChatText) StreamStatus(status string) string {
	return fmt.Sprintf(c.StreamStatusTemplate, status)
}

func (c ChatText) StreamThinking(delta string) string {
	return fmt.Sprintf(c.StreamThinkingTemplate, delta)
}

func (c ChatText) StreamToolStart(callID, toolName, payload string) string {
	return fmt.Sprintf(c.StreamToolStartTemplate, callID, toolName, payload)
}

func (c ChatText) StreamToolResult(callID, toolName, payload string) string {
	return fmt.Sprintf(c.StreamToolResultTemplate, callID, toolName, payload)
}

func (c ChatText) StreamBotLine(reply string) string {
	return fmt.Sprintf(c.StreamBotLineTemplate, reply)
}

func (c ChatText) StreamBotRewrite(reply string) string {
	return fmt.Sprintf(c.StreamBotRewriteTemplate, reply)
}
