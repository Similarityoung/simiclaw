package tx

import (
	"context"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func (r *RuntimeRepository) ListRunnable(ctx context.Context, limit int) ([]runtimemodel.WorkItem, error) {
	ids, err := r.db.ListRunnableEventIDs(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]runtimemodel.WorkItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, runtimemodel.WorkItem{
			Kind:     runtimemodel.WorkKindEvent,
			Identity: id,
			EventID:  id,
		})
	}
	return items, nil
}

func (r *RuntimeRepository) ClaimWork(ctx context.Context, work runtimemodel.WorkItem, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error) {
	eventID := work.EventID
	if eventID == "" {
		eventID = work.Identity
	}
	claimed, ok, err := r.db.ClaimEvent(ctx, eventID, runID, now)
	if err != nil || !ok {
		return runtimemodel.ClaimContext{}, ok, err
	}
	claimedWork := work
	if claimedWork.Kind == "" {
		claimedWork.Kind = runtimemodel.WorkKindEvent
	}
	if claimedWork.Identity == "" {
		claimedWork.Identity = claimed.Event.EventID
	}
	if claimedWork.EventID == "" {
		claimedWork.EventID = claimed.Event.EventID
	}
	channel := ""
	if claimed.Event.Source == "telegram" {
		channel = "telegram"
	}
	return runtimemodel.ClaimContext{
		Work:       claimedWork,
		Event:      claimed.Event,
		RunID:      claimed.RunID,
		RunMode:    claimed.RunMode,
		SessionKey: claimed.Event.SessionKey,
		SessionID:  claimed.Event.ActiveSessionID,
		Source:     claimed.Event.Source,
		Channel:    channel,
	}, true, nil
}
