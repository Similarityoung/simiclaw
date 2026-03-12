package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	clichannel "github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/internal/ui/messages"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type viewMode int

const (
	modeSelector viewMode = iota
	modeNaming
	modeChat
)

type chatMessage struct {
	Role    string
	Content string
}

type modelState struct {
	streams      common.IOStreams
	client       *client.Client
	opts         Options
	mode         viewMode
	width        int
	height       int
	sessions     []api.SessionRecord
	selectorIdx  int
	selectorBusy bool
	loading      bool
	sending      bool
	input        textarea.Model
	nameInput    textinput.Model
	viewport     viewport.Model
	messages     []chatMessage
	conversation string
	sessionKey   string
	sessionID    string
	status       string
	lastError    string
	lastUserText string
	seq          int64
	streamCh     <-chan tea.Msg
	cancelStream context.CancelFunc
	finalErr     error
}

type sessionsLoadedMsg struct {
	Items []api.SessionRecord
	Err   error
}

type sessionOpenedMsg struct {
	Session      *api.SessionRecord
	Conversation string
	Messages     []api.MessageRecord
	Err          error
}

type conversationCheckedMsg struct {
	Conversation string
	Available    bool
	Err          error
}

type streamStartedMsg struct {
	Updates <-chan tea.Msg
	Cancel  context.CancelFunc
}

type streamFrameMsg struct {
	Frame api.ChatStreamEvent
}

type streamDoneMsg struct{}

type streamErrorMsg struct {
	Err error
}

func newModel(streams common.IOStreams, cli *client.Client, opts Options) *modelState {
	input := textarea.New()
	input.Placeholder = messages.Chat.InputPlaceholder
	input.ShowLineNumbers = false
	input.SetHeight(4)
	input.Focus()
	input.Prompt = ""
	input.KeyMap.InsertNewline.SetKeys("ctrl+j")

	nameInput := textinput.New()
	nameInput.Placeholder = defaultConversationID()
	nameInput.CharLimit = 128

	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	m := &modelState{
		streams:      streams,
		client:       cli,
		opts:         opts,
		mode:         modeSelector,
		input:        input,
		nameInput:    nameInput,
		viewport:     vp,
		status:       messages.Chat.StatusLoadingSessions,
		seq:          time.Now().UTC().UnixMilli(),
		conversation: strings.TrimSpace(opts.Conversation),
		sessionKey:   strings.TrimSpace(opts.SessionKey),
	}
	if opts.NewSession {
		m.mode = modeChat
		if m.conversation == "" {
			m.conversation = defaultConversationID()
		}
		m.status = messages.Chat.StatusNewSessionPending
	}
	return m
}

func (m *modelState) Init() tea.Cmd {
	if m.opts.NewSession {
		if m.conversation != "" {
			m.loading = true
			m.status = messages.Chat.StatusCheckingConversation
			return checkConversationAvailableCmd(m.client, m.conversation)
		}
		m.syncViewport()
		return nil
	}
	if m.sessionKey != "" {
		m.loading = true
		m.status = messages.Chat.StatusLoadingHistory
		return openSessionCmd(m.client, m.sessionKey, m.opts.HistoryLimit)
	}
	if m.conversation != "" {
		m.loading = true
		m.status = messages.Chat.StatusLoadingHistory
		return openConversationCmd(m.client, m.conversation, m.opts.HistoryLimit)
	}
	m.selectorBusy = true
	return loadSessionsCmd(m.client)
}

func (m *modelState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case sessionsLoadedMsg:
		m.selectorBusy = false
		m.loading = false
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.status = messages.Chat.StatusLoadFailed
			return m, nil
		}
		m.sessions = msg.Items
		if len(m.sessions) == 0 {
			m.status = messages.Chat.StatusNoSessions
		} else {
			m.status = messages.Chat.LoadedSessions(len(m.sessions))
			if m.selectorIdx >= len(m.sessions) {
				m.selectorIdx = len(m.sessions) - 1
			}
		}
		return m, nil
	case sessionOpenedMsg:
		m.loading = false
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.status = messages.Chat.StatusLoadFailed
			return m, nil
		}
		m.mode = modeChat
		m.messages = nil
		if msg.Session != nil {
			m.conversation = msg.Session.ConversationID
			m.sessionKey = msg.Session.SessionKey
			m.sessionID = msg.Session.ActiveSessionID
			m.status = messages.Chat.OpenedSession(msg.Session.ConversationID)
		} else {
			m.conversation = msg.Conversation
			m.sessionKey = ""
			m.sessionID = ""
			m.status = messages.Chat.NewSession(msg.Conversation)
		}
		for _, item := range msg.Messages {
			if !item.Visible {
				continue
			}
			m.messages = append(m.messages, chatMessage{Role: item.Role, Content: item.Content})
		}
		m.lastError = ""
		m.syncViewport()
		return m, nil
	case conversationCheckedMsg:
		m.loading = false
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.status = messages.Chat.StatusCheckConversationFailed
			return m, nil
		}
		if !msg.Available {
			m.mode = modeNaming
			m.messages = nil
			m.sessionKey = ""
			m.sessionID = ""
			m.nameInput.SetValue(msg.Conversation)
			m.lastError = ""
			m.status = messages.Chat.ConversationExists(msg.Conversation)
			return m, m.nameInput.Focus()
		}
		m.mode = modeChat
		m.messages = nil
		m.conversation = msg.Conversation
		m.sessionKey = ""
		m.sessionID = ""
		m.lastError = ""
		m.status = messages.Chat.NewSession(msg.Conversation)
		m.syncViewport()
		return m, m.input.Focus()
	case streamStartedMsg:
		m.cancelActiveStream()
		m.streamCh = msg.Updates
		m.cancelStream = msg.Cancel
		m.sending = true
		m.status = messages.Chat.StatusMessageSentWait
		return m, waitForAsyncMsg(m.streamCh)
	case streamFrameMsg:
		m.applyStreamFrame(msg.Frame)
		if isTerminalFrame(msg.Frame) {
			m.sending = false
			m.cancelActiveStream()
			return m, nil
		}
		return m, waitForAsyncMsg(m.streamCh)
	case streamErrorMsg:
		m.sending = false
		m.cancelActiveStream()
		m.lastError = msg.Err.Error()
		m.status = messages.Chat.StatusSendFailed
		return m, nil
	case streamDoneMsg:
		m.sending = false
		m.cancelActiveStream()
		return m, nil
	default:
		var cmd tea.Cmd
		switch m.mode {
		case modeNaming:
			m.nameInput, cmd = m.nameInput.Update(msg)
		case modeChat:
			m.input, cmd = m.input.Update(msg)
			m.syncViewport()
		}
		return m, cmd
	}
}

func (m *modelState) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSelector:
		return m.handleSelectorKey(msg)
	case modeNaming:
		return m.handleNamingKey(msg)
	default:
		return m.handleChatKey(msg)
	}
}

func (m *modelState) handleSelectorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selectorIdx > 0 {
			m.selectorIdx--
		}
	case "down", "j":
		if m.selectorIdx < len(m.sessions)-1 {
			m.selectorIdx++
		}
	case "ctrl+n":
		m.mode = modeNaming
		m.nameInput.SetValue("")
		m.nameInput.Placeholder = defaultConversationID()
		return m, m.nameInput.Focus()
	case "r":
		m.selectorBusy = true
		m.status = messages.Chat.StatusRefreshingSessions
		return m, loadSessionsCmd(m.client)
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		m.loading = true
		m.status = messages.Chat.StatusLoadingHistory
		return m, openSessionCmd(m.client, m.sessions[m.selectorIdx].SessionKey, m.opts.HistoryLimit)
	}
	return m, nil
}

func (m *modelState) handleNamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeSelector
		return m, nil
	case "enter":
		conversation := strings.TrimSpace(m.nameInput.Value())
		if conversation == "" {
			conversation = defaultConversationID()
		}
		m.loading = true
		m.lastError = ""
		m.status = messages.Chat.StatusCheckingConversation
		return m, checkConversationAvailableCmd(m.client, conversation)
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *modelState) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cancelActiveStream()
		return m, tea.Quit
	case "ctrl+o":
		if m.sending {
			return m, nil
		}
		m.mode = modeSelector
		m.selectorBusy = true
		m.status = messages.Chat.StatusLoadingSessionList
		return m, loadSessionsCmd(m.client)
	case "ctrl+n":
		if m.sending {
			return m, nil
		}
		m.mode = modeNaming
		m.nameInput.SetValue("")
		m.nameInput.Placeholder = defaultConversationID()
		return m, m.nameInput.Focus()
	case "ctrl+r":
		if m.sending || strings.TrimSpace(m.lastUserText) == "" {
			return m, nil
		}
		return m, m.startSend(strings.TrimSpace(m.lastUserText))
	case "enter":
		if m.sending {
			return m, nil
		}
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.input.Reset()
		return m, m.startSend(text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *modelState) startSend(text string) tea.Cmd {
	m.lastUserText = text
	m.messages = append(m.messages, chatMessage{Role: "user", Content: text})
	m.syncViewport()
	conversation := m.conversation
	if conversation == "" {
		conversation = defaultConversationID()
		m.conversation = conversation
	}
	req := clichannel.BuildIngestRequest(conversation, fixedParticipantID, m.seq, text)
	m.seq++
	return startSendCmd(m.client, req, m.opts.NoStream)
}

func (m *modelState) applyStreamFrame(frame api.ChatStreamEvent) {
	if frame.IngestResponse != nil {
		if frame.IngestResponse.SessionKey != "" {
			m.sessionKey = frame.IngestResponse.SessionKey
		}
		if frame.IngestResponse.ActiveSessionID != "" {
			m.sessionID = frame.IngestResponse.ActiveSessionID
		}
	}
	if frame.EventRecord != nil {
		if frame.EventRecord.SessionKey != "" {
			m.sessionKey = frame.EventRecord.SessionKey
		}
		if frame.EventRecord.SessionID != "" {
			m.sessionID = frame.EventRecord.SessionID
		}
	}

	switch frame.Type {
	case api.ChatStreamEventAccepted:
		m.status = messages.Chat.StatusRequestAccepted
	case api.ChatStreamEventStatus:
		statusText := strings.TrimSpace(frame.Message)
		if statusText == "" {
			statusText = strings.TrimSpace(frame.Status)
		}
		if statusText == "" {
			statusText = messages.Chat.StatusProcessing
		}
		m.status = statusText
	case api.ChatStreamEventReasoningDelta:
		m.status = messages.Chat.StatusReasoning
	case api.ChatStreamEventTextDelta:
		m.status = messages.Chat.StatusStreamingReply
		m.appendAssistantDelta(frame.Delta)
	case api.ChatStreamEventToolStart:
		m.status = messages.Chat.RunningTool(nonEmpty(frame.ToolName, frame.ToolCallID))
	case api.ChatStreamEventToolResult:
		if frame.Error != nil {
			m.status = messages.Chat.ToolFailed(nonEmpty(frame.ToolName, frame.ToolCallID))
		} else {
			m.status = messages.Chat.ToolCompleted(nonEmpty(frame.ToolName, frame.ToolCallID))
		}
	case api.ChatStreamEventDone, api.ChatStreamEventError:
		m.applyTerminalRecord(frame)
	}
	m.syncViewport()
}

func (m *modelState) applyTerminalRecord(frame api.ChatStreamEvent) {
	rec := frame.EventRecord
	if rec == nil {
		if frame.Error != nil {
			m.lastError = frame.Error.Code + ": " + frame.Error.Message
		}
		if frame.Type == api.ChatStreamEventError {
			m.status = messages.Chat.StatusRunFailed
		} else {
			m.status = messages.Chat.StatusDone
		}
		return
	}

	if rec.SessionKey != "" {
		m.sessionKey = rec.SessionKey
	}
	if rec.SessionID != "" {
		m.sessionID = rec.SessionID
	}
	m.finalizeAssistant(rec.AssistantReply)
	switch rec.Status {
	case model.EventStatusFailed:
		m.status = messages.Chat.StatusRunFailed
		if rec.Error != nil {
			m.lastError = rec.Error.Code + ": " + rec.Error.Message
		}
	default:
		m.status = messages.Chat.StatusDone
		m.lastError = ""
	}
}

func (m *modelState) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if len(m.messages) == 0 || m.messages[len(m.messages)-1].Role != "assistant" {
		m.messages = append(m.messages, chatMessage{Role: "assistant", Content: delta})
		return
	}
	m.messages[len(m.messages)-1].Content += delta
}

func (m *modelState) finalizeAssistant(reply string) {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return
	}
	if len(m.messages) == 0 || m.messages[len(m.messages)-1].Role != "assistant" {
		m.messages = append(m.messages, chatMessage{Role: "assistant", Content: reply})
		return
	}
	m.messages[len(m.messages)-1].Content = reply
}

func (m *modelState) cancelActiveStream() {
	if m.cancelStream != nil {
		m.cancelStream()
		m.cancelStream = nil
	}
	m.streamCh = nil
}

func (m *modelState) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	inputHeight := 5
	m.input.SetWidth(max(20, m.width-4))
	m.input.SetHeight(inputHeight - 1)
	bodyHeight := m.height - inputHeight - 5
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	m.viewport.Width = m.width
	m.viewport.Height = bodyHeight
	m.syncViewport()
}

func (m *modelState) syncViewport() {
	if m.mode != modeChat {
		return
	}
	content := renderMessages(m.messages)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *modelState) View() string {
	switch m.mode {
	case modeSelector:
		return m.renderSelector()
	case modeNaming:
		return m.renderNaming()
	default:
		return m.renderChat()
	}
}

func (m *modelState) renderSelector() string {
	var b strings.Builder
	b.WriteString(messages.Chat.SelectorTitle)
	b.WriteString(messages.Chat.SelectorHelp)
	if m.selectorBusy {
		b.WriteString(messages.Chat.SelectorLoading)
	} else if len(m.sessions) == 0 {
		b.WriteString(messages.Chat.SelectorEmpty)
	} else {
		for i, item := range m.sessions {
			cursor := "  "
			if i == m.selectorIdx {
				cursor = "> "
			}
			b.WriteString(messages.Chat.SelectorItem(cursor, item.ConversationID, item.MessageCount, item.LastModel, item.LastActivityAt.Format(time.RFC3339)))
		}
	}
	b.WriteString("\n")
	b.WriteString(m.renderStatus())
	return b.String()
}

func (m *modelState) renderNaming() string {
	return messages.Chat.NamingView(m.nameInput.View(), m.renderStatus())
}

func (m *modelState) renderChat() string {
	header := messages.Chat.Header(nonEmpty(m.conversation, messages.Chat.ConversationMissing), nonEmpty(m.sessionKey, messages.Chat.SessionPending))
	help := messages.Chat.Help
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		strings.Repeat("─", max(10, m.width-1)),
		m.viewport.View(),
		strings.Repeat("─", max(10, m.width-1)),
		m.input.View(),
		help,
		m.renderStatus(),
	)
}

func (m *modelState) renderStatus() string {
	parts := []string{m.status}
	if m.lastError != "" {
		parts = append(parts, messages.Chat.StatusErrorPrefix+m.lastError)
	}
	return strings.Join(parts, "  |  ")
}

func renderMessages(items []chatMessage) string {
	if len(items) == 0 {
		return messages.Chat.NoMessages
	}
	var parts []string
	for _, msg := range items {
		var prefix string
		switch msg.Role {
		case "user":
			prefix = messages.Chat.MessagePrefixUser
		case "assistant":
			prefix = messages.Chat.MessagePrefixAssistant
		default:
			prefix = msg.Role
		}
		parts = append(parts, fmt.Sprintf("%s> %s", prefix, msg.Content))
	}
	return strings.Join(parts, "\n\n")
}

func loadSessionsCmd(cli *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		page, err := cli.ListSessions(ctx, "", "", "", 30)
		if err != nil {
			return sessionsLoadedMsg{Err: err}
		}
		return sessionsLoadedMsg{Items: page.Items}
	}
}

func openSessionCmd(cli *client.Client, sessionKey string, historyLimit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		session, err := cli.GetSession(ctx, sessionKey)
		if err != nil {
			return sessionOpenedMsg{Err: err}
		}
		history, err := cli.GetSessionHistory(ctx, sessionKey, "", historyLimit, true)
		if err != nil {
			return sessionOpenedMsg{Err: err}
		}
		return sessionOpenedMsg{Session: &session, Messages: history.Items}
	}
}

func openConversationCmd(cli *client.Client, conversation string, historyLimit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		session, err := findConversationSession(ctx, cli, conversation)
		if err != nil {
			return sessionOpenedMsg{Err: err}
		}
		if session == nil {
			return sessionOpenedMsg{Conversation: conversation}
		}
		history, err := cli.GetSessionHistory(ctx, session.SessionKey, "", historyLimit, true)
		if err != nil {
			return sessionOpenedMsg{Err: err}
		}
		return sessionOpenedMsg{Session: session, Messages: history.Items}
	}
}

func checkConversationAvailableCmd(cli *client.Client, conversation string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		session, err := findConversationSession(ctx, cli, conversation)
		if err != nil {
			return conversationCheckedMsg{Conversation: conversation, Err: err}
		}
		return conversationCheckedMsg{Conversation: conversation, Available: session == nil}
	}
}

func findConversationSession(ctx context.Context, cli *client.Client, conversation string) (*api.SessionRecord, error) {
	conversation = strings.TrimSpace(conversation)
	if conversation == "" {
		return nil, nil
	}
	cursor := ""
	for {
		page, err := cli.ListSessions(ctx, "", conversation, cursor, 200)
		if err != nil {
			return nil, err
		}
		for i := range page.Items {
			item := page.Items[i]
			if item.ConversationID != conversation {
				continue
			}
			if item.ChannelType != "dm" || item.ParticipantID != fixedParticipantID {
				continue
			}
			matched := item
			return &matched, nil
		}
		if page.NextCursor == "" {
			return nil, nil
		}
		cursor = page.NextCursor
	}
}

func startSendCmd(cli *client.Client, req api.IngestRequest, noStream bool) tea.Cmd {
	return func() tea.Msg {
		updates := make(chan tea.Msg, 64)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(updates)
			emit := func(event api.ChatStreamEvent) error {
				updates <- streamFrameMsg{Frame: event}
				return nil
			}

			var err error
			if noStream {
				resp, ingestErr := cli.Ingest(ctx, req)
				if ingestErr != nil {
					updates <- streamErrorMsg{Err: ingestErr}
					return
				}
				if err = emit(api.ChatStreamEvent{
					Type:                  api.ChatStreamEventAccepted,
					EventID:               resp.EventID,
					At:                    time.Now().UTC(),
					StreamProtocolVersion: api.ChatStreamProtocolVersion,
					IngestResponse:        &resp,
				}); err != nil {
					updates <- streamErrorMsg{Err: err}
					return
				}
				rec, waitErr := cli.WaitEvent(ctx, resp.EventID)
				if waitErr != nil {
					updates <- streamErrorMsg{Err: waitErr}
					return
				}
				if err = emit(eventToTerminalFrame(rec)); err != nil {
					updates <- streamErrorMsg{Err: err}
					return
				}
			} else {
				_, err = cli.StreamChat(ctx, req, func(event api.ChatStreamEvent) error {
					updates <- streamFrameMsg{Frame: event}
					return nil
				})
				if err != nil {
					updates <- streamErrorMsg{Err: err}
					return
				}
			}
			updates <- streamDoneMsg{}
		}()
		return streamStartedMsg{Updates: updates, Cancel: cancel}
	}
}

func waitForAsyncMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamDoneMsg{}
		}
		return msg
	}
}

func eventToTerminalFrame(rec api.EventRecord) api.ChatStreamEvent {
	frame := api.ChatStreamEvent{
		Type:        api.ChatStreamEventDone,
		EventID:     rec.EventID,
		At:          time.Now().UTC(),
		EventRecord: &rec,
		Error:       rec.Error,
	}
	if rec.Status == model.EventStatusFailed {
		frame.Type = api.ChatStreamEventError
	}
	return frame
}

func isTerminalFrame(frame api.ChatStreamEvent) bool {
	return frame.IsTerminal()
}

func defaultConversationID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("cli_%s_%09d", now.Format("20060102T150405Z"), now.Nanosecond())
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
