package lanes

import (
	"strings"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

type Key string

const (
	sessionPrefix = "session:"
	eventPrefix   = "event:"
	globalKey     = "work:global"
)

func Resolve(work runtimemodel.WorkItem) Key {
	if key := strings.TrimSpace(work.LaneKey); key != "" {
		return Key(key)
	}
	if sessionKey := strings.TrimSpace(work.SessionKey); sessionKey != "" {
		return Key(sessionPrefix + sessionKey)
	}
	if eventID := strings.TrimSpace(work.EventID); eventID != "" {
		return Key(eventPrefix + eventID)
	}
	return Key(globalKey)
}
