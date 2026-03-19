package chat

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

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
