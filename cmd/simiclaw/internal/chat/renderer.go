package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type streamRenderer struct {
	out        io.Writer
	lineOpen   bool
	streamed   strings.Builder
	lastStatus string
}

func newStreamRenderer(out io.Writer) *streamRenderer {
	return &streamRenderer{out: out}
}

func (r *streamRenderer) HandleStreamEvent(event model.ChatStreamEvent) error {
	switch event.Type {
	case model.ChatStreamEventAccepted:
		return r.openBotLine()
	case model.ChatStreamEventStatus:
		statusText := strings.TrimSpace(event.Message)
		if statusText == "" {
			statusText = strings.TrimSpace(event.Status)
		}
		if statusText == "" || statusText == r.lastStatus {
			return nil
		}
		r.lastStatus = statusText
		if err := r.closeBotLine(); err != nil {
			return err
		}
		_, err := fmt.Fprintf(r.out, "status> %s\n", statusText)
		return err
	case model.ChatStreamEventReasoningDelta:
		if strings.TrimSpace(event.Delta) == "" {
			return nil
		}
		if err := r.closeBotLine(); err != nil {
			return err
		}
		_, err := fmt.Fprintf(r.out, "think> %s\n", event.Delta)
		return err
	case model.ChatStreamEventTextDelta:
		if err := r.openBotLine(); err != nil {
			return err
		}
		if _, err := fmt.Fprint(r.out, event.Delta); err != nil {
			return err
		}
		r.streamed.WriteString(event.Delta)
		return nil
	case model.ChatStreamEventToolStart:
		if err := r.closeBotLine(); err != nil {
			return err
		}
		_, err := fmt.Fprintf(r.out, "tool> [%s] %s %s\n", event.ToolCallID, event.ToolName, formatToolPayload(event.Args, event.Truncated))
		return err
	case model.ChatStreamEventToolResult:
		if err := r.closeBotLine(); err != nil {
			return err
		}
		payload := formatToolPayload(event.Result, event.Truncated)
		if event.Error != nil {
			payload = event.Error.Code + ": " + event.Error.Message
		}
		_, err := fmt.Fprintf(r.out, "tool< [%s] %s %s\n", event.ToolCallID, event.ToolName, payload)
		return err
	default:
		return nil
	}
}

func (r *streamRenderer) Finish(rec model.EventRecord) error {
	reply := rec.AssistantReply
	if rec.Status == model.EventStatusFailed {
		return r.closeBotLine()
	}
	if reply == "" {
		if r.streamed.Len() == 0 {
			if err := r.openBotLine(); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.out, "(no reply)"); err != nil {
				return err
			}
		}
		return r.closeBotLine()
	}
	if r.streamed.String() == reply {
		return r.closeBotLine()
	}
	if !r.lineOpen {
		_, err := fmt.Fprintf(r.out, "bot> %s\n", reply)
		return err
	}
	if _, err := fmt.Fprintf(r.out, "\r\033[2Kbot> %s\n", reply); err != nil {
		return err
	}
	r.lineOpen = false
	return nil
}

func (r *streamRenderer) Abort() {
	_ = r.closeBotLine()
}

func (r *streamRenderer) openBotLine() error {
	if r.lineOpen {
		return nil
	}
	_, err := fmt.Fprint(r.out, "bot> ")
	if err == nil {
		r.lineOpen = true
	}
	return err
}

func (r *streamRenderer) closeBotLine() error {
	if !r.lineOpen {
		return nil
	}
	r.lineOpen = false
	_, err := fmt.Fprintln(r.out)
	return err
}

func formatToolPayload(v map[string]any, truncated bool) string {
	if len(v) == 0 {
		if truncated {
			return "{} [truncated]"
		}
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	if truncated {
		return string(b) + " [truncated]"
	}
	return string(b)
}
