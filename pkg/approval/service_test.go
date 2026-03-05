package approval

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

func TestDecideApproveIdempotentAndRejectConflict(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	svc, err := NewService(workspace, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	now := time.Now().UTC()
	rec, err := svc.Create(model.CreateApprovalRequest{
		RunID:           "run_1",
		SessionKey:      "sk:1",
		ActiveSessionID: "s_1",
		ConversationID:  "conv_1",
		ExpiresAt:       now.Add(10 * time.Minute).Format(time.RFC3339),
		Summary:         "patch request",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_1", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target_path": "workflows/a.yaml"}}},
	}, now)
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	approved, changed, conflict, notFound, err := svc.Decide(rec.ApprovalID, true, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u1"},
		Note:  "ok",
	}, now)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !changed || conflict || notFound || approved.Status != model.ApprovalStatusApproved {
		t.Fatalf("unexpected approve result: changed=%t conflict=%t notFound=%t rec=%+v", changed, conflict, notFound, approved)
	}

	approvedAgain, changed, conflict, notFound, err := svc.Decide(rec.ApprovalID, true, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u1"},
		Note:  "retry",
	}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("approve again: %v", err)
	}
	if changed || conflict || notFound || approvedAgain.Status != model.ApprovalStatusApproved {
		t.Fatalf("duplicate approve should be idempotent: changed=%t conflict=%t notFound=%t rec=%+v", changed, conflict, notFound, approvedAgain)
	}

	_, changed, conflict, notFound, err = svc.Decide(rec.ApprovalID, false, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u2"},
		Note:  "reject",
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("reject after approve: %v", err)
	}
	if changed || !conflict || notFound {
		t.Fatalf("expected conflict on reject after approve: changed=%t conflict=%t notFound=%t", changed, conflict, notFound)
	}
}

func TestDecideExpiredBecomesConflict(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	svc, err := NewService(workspace, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	now := time.Now().UTC()
	rec, err := svc.Create(model.CreateApprovalRequest{
		RunID:           "run_2",
		SessionKey:      "sk:2",
		ActiveSessionID: "s_2",
		ConversationID:  "conv_2",
		ExpiresAt:       now.Add(-time.Minute).Format(time.RFC3339),
		Summary:         "expired patch",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_2", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target_path": "workflows/a.yaml"}}},
	}, now.Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	updated, changed, conflict, notFound, err := svc.Decide(rec.ApprovalID, true, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u1"},
	}, now)
	if err != nil {
		t.Fatalf("approve expired: %v", err)
	}
	if changed || !conflict || notFound {
		t.Fatalf("expired approval should conflict: changed=%t conflict=%t notFound=%t", changed, conflict, notFound)
	}
	if updated.Status != model.ApprovalStatusExpired {
		t.Fatalf("expected status expired, got %s", updated.Status)
	}
}

func TestApproveRetryCanRepublishDecisionEvent(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	eventBus := bus.NewMessageBus(4)
	svc, err := NewService(workspace, eventBus)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	now := time.Now().UTC()
	rec, err := svc.Create(model.CreateApprovalRequest{
		RunID:           "run_3",
		SessionKey:      "sk:3",
		ActiveSessionID: "s_3",
		ConversationID:  "conv_3",
		ExpiresAt:       now.Add(10 * time.Minute).Format(time.RFC3339),
		Summary:         "retry publish",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_3", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target_path": "workflows/a.yaml"}}},
	}, now)
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	_, changed, conflict, notFound, err := svc.Decide(rec.ApprovalID, true, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u3"},
		Note:  "approve",
	}, now)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !changed || conflict || notFound {
		t.Fatalf("unexpected first approve state: changed=%t conflict=%t notFound=%t", changed, conflict, notFound)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.PublishDecisionEventOnce(cancelCtx, rec.ApprovalID, now.Add(time.Second)); err == nil {
		t.Fatalf("expected publish failure from canceled context")
	}
	stored, ok, err := svc.Get(rec.ApprovalID)
	if err != nil || !ok {
		t.Fatalf("get approval failed: ok=%t err=%v", ok, err)
	}
	if stored.DecisionEventPublishedAt != nil {
		t.Fatalf("decision event should not be marked published on failure")
	}

	_, changed, conflict, notFound, err = svc.Decide(rec.ApprovalID, true, model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u3"},
		Note:  "retry approve",
	}, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("approve retry: %v", err)
	}
	if !changed || conflict || notFound {
		t.Fatalf("retry approve should request republish: changed=%t conflict=%t notFound=%t", changed, conflict, notFound)
	}

	updated, err := svc.PublishDecisionEventOnce(context.Background(), rec.ApprovalID, now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("publish retry: %v", err)
	}
	if updated.DecisionEventPublishedAt == nil {
		t.Fatalf("decision event should be marked published")
	}
	if eventBus.InboundDepth() != 1 {
		t.Fatalf("expected one decision event queued, got %d", eventBus.InboundDepth())
	}

	again, err := svc.PublishDecisionEventOnce(context.Background(), rec.ApprovalID, now.Add(4*time.Second))
	if err != nil {
		t.Fatalf("publish duplicate: %v", err)
	}
	if again.DecisionEventPublishedAt == nil {
		t.Fatalf("published marker should remain set")
	}
	if eventBus.InboundDepth() != 1 {
		t.Fatalf("duplicate publish should not enqueue new event, got %d", eventBus.InboundDepth())
	}
}
