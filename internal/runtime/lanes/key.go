package lanes

import (
	"strings"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

type Key string

const (
	sessionPrefix   = "session:"
	eventPrefix     = "event:"
	outboxPrefix    = "outbox:"
	scheduledPrefix = "job:"
	recoveryPrefix  = "recovery:"
	workPrefix      = "work:"
	globalKey       = "work:global"
)

func Resolve(work runtimemodel.WorkItem) Key {
	if key := strings.TrimSpace(work.LaneKey); key != "" {
		return Key(key)
	}
	if sessionKey := strings.TrimSpace(work.SessionKey); sessionKey != "" {
		return Key(sessionPrefix + sessionKey)
	}

	switch work.Kind {
	case runtimemodel.WorkKindOutbox:
		if id := firstNonEmpty(work.OutboxID, work.Identity); id != "" {
			return Key(outboxPrefix + id)
		}
	case runtimemodel.WorkKindScheduledJob:
		if id := firstNonEmpty(work.JobID, work.Identity); id != "" {
			return Key(scheduledPrefix + id)
		}
	case runtimemodel.WorkKindRecovery:
		if id := firstNonEmpty(work.EventID, work.Identity); id != "" {
			return Key(recoveryPrefix + id)
		}
	case runtimemodel.WorkKindEvent:
		if id := firstNonEmpty(work.EventID, work.Identity); id != "" {
			return Key(eventPrefix + id)
		}
	}

	if id := strings.TrimSpace(work.Identity); id != "" {
		return Key(workPrefix + id)
	}
	return Key(globalKey)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
