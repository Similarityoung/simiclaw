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

	clichannel "github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/pkg/model"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
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
	sessions     []model.SessionRecord
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
	Items []model.SessionRecord
	Err   error
}

type sessionOpenedMsg struct {
	Session      *model.SessionRecord
	Conversation string
	Messages     []model.MessageRecord
	Err          error
}

type streamStartedMsg struct {
	Updates <-chan tea.Msg
	Cancel  context.CancelFunc
}

type streamFrameMsg struct {
	Frame model.ChatStreamEvent
}

type streamDoneMsg struct{}

type streamErrorMsg struct {
	Err error
}

func newModel(streams common.IOStreams, cli *client.Client, opts Options) *modelState {
	input := textarea.New()
	input.Placeholder = "输入消息，Enter 发送，Ctrl+J 换行"
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
		status:       "加载会话中…",
		seq:          time.Now().UTC().UnixMilli(),
		conversation: strings.TrimSpace(opts.Conversation),
		sessionKey:   strings.TrimSpace(opts.SessionKey),
	}
	if opts.NewSession {
		m.mode = modeChat
		if m.conversation == "" {
			m.conversation = defaultConversationID()
		}
		m.status = "新会话，发送第一条消息后会创建远端 session"
	}
	return m
}

func (m *modelState) Init() tea.Cmd {
	if m.opts.NewSession {
		m.syncViewport()
		return nil
	}
	if m.sessionKey != "" {
		m.loading = true
		m.status = "加载会话历史中…"
		return openSessionCmd(m.client, m.sessionKey, m.opts.HistoryLimit)
	}
	if m.conversation != "" {
		m.mode = modeChat
		m.status = "使用指定 conversation，发送第一条消息后会创建远端 session"
		m.syncViewport()
		return nil
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
			m.status = "加载会话失败"
			return m, nil
		}
		m.sessions = msg.Items
		if len(m.sessions) == 0 {
			m.status = "暂无会话，按 Ctrl+N 新建"
		} else {
			m.status = fmt.Sprintf("已加载 %d 个会话", len(m.sessions))
			if m.selectorIdx >= len(m.sessions) {
				m.selectorIdx = len(m.sessions) - 1
			}
		}
		return m, nil
	case sessionOpenedMsg:
		m.loading = false
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.status = "加载会话失败"
			return m, nil
		}
		m.mode = modeChat
		m.messages = nil
		if msg.Session != nil {
			m.conversation = msg.Session.ConversationID
			m.sessionKey = msg.Session.SessionKey
			m.sessionID = msg.Session.ActiveSessionID
			m.status = fmt.Sprintf("已打开会话 %s", msg.Session.ConversationID)
		} else {
			m.conversation = msg.Conversation
			m.sessionKey = ""
			m.sessionID = ""
			m.status = fmt.Sprintf("新会话 %s", msg.Conversation)
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
	case streamStartedMsg:
		m.cancelActiveStream()
		m.streamCh = msg.Updates
		m.cancelStream = msg.Cancel
		m.sending = true
		m.status = "消息已发送，等待回复…"
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
		m.status = "发送失败"
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
		m.status = "刷新会话中…"
		return m, loadSessionsCmd(m.client)
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		m.loading = true
		m.status = "加载会话历史中…"
		return m, openSessionCmd(m.client, m.sessions[m.selectorIdx].SessionKey, m.opts.HistoryLimit)
	}
	return m, nil
}

func (m *modelState) handleNamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.mode = modeChat
		m.messages = nil
		m.conversation = conversation
		m.sessionKey = ""
		m.sessionID = ""
		m.status = fmt.Sprintf("新会话 %s", conversation)
		m.syncViewport()
		return m, m.input.Focus()
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
		m.status = "加载会话列表中…"
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

func (m *modelState) applyStreamFrame(frame model.ChatStreamEvent) {
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
	case model.ChatStreamEventAccepted:
		m.status = "请求已接收"
	case model.ChatStreamEventStatus:
		statusText := strings.TrimSpace(frame.Message)
		if statusText == "" {
			statusText = strings.TrimSpace(frame.Status)
		}
		if statusText == "" {
			statusText = "处理中…"
		}
		m.status = statusText
	case model.ChatStreamEventReasoningDelta:
		m.status = "推理中…"
	case model.ChatStreamEventTextDelta:
		m.status = "回复流式输出中…"
		m.appendAssistantDelta(frame.Delta)
	case model.ChatStreamEventToolStart:
		m.status = fmt.Sprintf("执行工具：%s", nonEmpty(frame.ToolName, frame.ToolCallID))
	case model.ChatStreamEventToolResult:
		if frame.Error != nil {
			m.status = fmt.Sprintf("工具失败：%s", nonEmpty(frame.ToolName, frame.ToolCallID))
		} else {
			m.status = fmt.Sprintf("工具完成：%s", nonEmpty(frame.ToolName, frame.ToolCallID))
		}
	case model.ChatStreamEventDone, model.ChatStreamEventError:
		m.applyTerminalRecord(frame)
	}
	m.syncViewport()
}

func (m *modelState) applyTerminalRecord(frame model.ChatStreamEvent) {
	rec := frame.EventRecord
	if rec == nil {
		if frame.Error != nil {
			m.lastError = frame.Error.Code + ": " + frame.Error.Message
		}
		if frame.Type == model.ChatStreamEventError {
			m.status = "运行失败"
		} else {
			m.status = "完成"
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
		m.status = "运行失败"
		if rec.Error != nil {
			m.lastError = rec.Error.Code + ": " + rec.Error.Message
		}
	default:
		m.status = "完成"
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
	b.WriteString("SimiClaw Chat\n")
	b.WriteString("选择会话（↑/↓ 或 j/k，Enter 打开，Ctrl+N 新建，r 刷新，Ctrl+C 退出）\n\n")
	if m.selectorBusy {
		b.WriteString("加载中…\n")
	} else if len(m.sessions) == 0 {
		b.WriteString("暂无会话\n")
	} else {
		for i, item := range m.sessions {
			cursor := "  "
			if i == m.selectorIdx {
				cursor = "> "
			}
			b.WriteString(fmt.Sprintf("%s%s  ·  %d 条消息  ·  %s  ·  %s\n", cursor, item.ConversationID, item.MessageCount, item.LastModel, item.LastActivityAt.Format(time.RFC3339)))
		}
	}
	b.WriteString("\n")
	b.WriteString(m.renderStatus())
	return b.String()
}

func (m *modelState) renderNaming() string {
	return fmt.Sprintf(
		"新建会话\n\n输入 conversation_id（留空使用默认值）\n\n%s\n\nEnter 确认，Esc 返回，Ctrl+C 退出\n\n%s",
		m.nameInput.View(),
		m.renderStatus(),
	)
}

func (m *modelState) renderChat() string {
	header := fmt.Sprintf("SimiClaw Chat  ·  conversation=%s  ·  session=%s", nonEmpty(m.conversation, "(未创建)"), nonEmpty(m.sessionKey, "(待首条消息创建)"))
	help := "Enter 发送  ·  Ctrl+J 换行  ·  Ctrl+R 重发  ·  Ctrl+O 会话列表  ·  Ctrl+N 新会话  ·  Ctrl+C 退出"
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
		parts = append(parts, "error="+m.lastError)
	}
	return strings.Join(parts, "  |  ")
}

func renderMessages(messages []chatMessage) string {
	if len(messages) == 0 {
		return "还没有消息，输入第一条开始对话。"
	}
	var parts []string
	for _, msg := range messages {
		prefix := "bot"
		switch msg.Role {
		case "user":
			prefix = "you"
		case "assistant":
			prefix = "bot"
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

func startSendCmd(cli *client.Client, req model.IngestRequest, noStream bool) tea.Cmd {
	return func() tea.Msg {
		updates := make(chan tea.Msg, 64)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(updates)
			emit := func(event model.ChatStreamEvent) error {
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
				if err = emit(model.ChatStreamEvent{
					Type:                  model.ChatStreamEventAccepted,
					EventID:               resp.EventID,
					At:                    time.Now().UTC(),
					StreamProtocolVersion: model.ChatStreamProtocolVersion,
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
				_, err = cli.StreamChat(ctx, req, func(event model.ChatStreamEvent) error {
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

func eventToTerminalFrame(rec model.EventRecord) model.ChatStreamEvent {
	frame := model.ChatStreamEvent{
		Type:        model.ChatStreamEventDone,
		EventID:     rec.EventID,
		At:          time.Now().UTC(),
		EventRecord: &rec,
		Error:       rec.Error,
	}
	if rec.Status == model.EventStatusFailed {
		frame.Type = model.ChatStreamEventError
	}
	return frame
}

func isTerminalFrame(frame model.ChatStreamEvent) bool {
	return frame.IsTerminal()
}

func defaultConversationID() string {
	return "cli_" + time.Now().UTC().Format("20060102T150405Z")
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
