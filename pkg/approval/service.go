package approval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const defaultAutoApprovalTTL = 24 * time.Hour

type Service struct {
	workspace string
	repo      *Repo
	patcher   *PatchExecutor
	bus       *bus.MessageBus
}

func NewService(workspace string, eventBus *bus.MessageBus) (*Service, error) {
	patcher, err := NewPatchExecutor(workspace)
	if err != nil {
		return nil, err
	}
	return &Service{
		workspace: workspace,
		repo:      NewRepo(workspace),
		patcher:   patcher,
		bus:       eventBus,
	}, nil
}

func (s *Service) Create(req model.CreateApprovalRequest, now time.Time) (model.ApprovalRecord, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(req.RunID) == "" {
		return model.ApprovalRecord{}, fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(req.SessionKey) == "" {
		return model.ApprovalRecord{}, fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(req.ActiveSessionID) == "" {
		return model.ApprovalRecord{}, fmt.Errorf("active_session_id is required")
	}
	if strings.TrimSpace(req.ConversationID) == "" {
		return model.ApprovalRecord{}, fmt.Errorf("conversation_id is required")
	}
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		return model.ApprovalRecord{}, fmt.Errorf("invalid expires_at")
	}
	risk := parseRisk(req.Risk)
	rec := model.ApprovalRecord{
		ApprovalID:     fmt.Sprintf("apr_%d", now.UnixNano()),
		Status:         model.ApprovalStatusPending,
		Risk:           risk,
		SessionKey:     req.SessionKey,
		SessionID:      req.ActiveSessionID,
		RunID:          req.RunID,
		ConversationID: req.ConversationID,
		Summary:        strings.TrimSpace(req.Summary),
		Actions:        req.Actions,
		CreatedAt:      now,
		ExpiresAt:      expiresAt.UTC(),
		UpdatedAt:      now,
	}
	if rec.Summary == "" {
		rec.Summary = "需要审批的高风险动作"
	}
	if err := s.repo.Create(rec); err != nil {
		return model.ApprovalRecord{}, err
	}
	_ = appendEvolution(s.workspace, "approval proposal", []string{
		"approval_id=" + rec.ApprovalID,
		"run_id=" + rec.RunID,
		"status=pending",
	}, now)
	return rec, nil
}

func (s *Service) CreateAuto(runID, sessionKey, sessionID, conversationID string, actions []model.Action, summary string, now time.Time) (model.ApprovalRecord, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	req := model.CreateApprovalRequest{
		RunID:           runID,
		SessionKey:      sessionKey,
		ActiveSessionID: sessionID,
		ConversationID:  conversationID,
		ExpiresAt:       now.Add(defaultAutoApprovalTTL).UTC().Format(time.RFC3339),
		Summary:         strings.TrimSpace(summary),
		Risk:            string(model.ApprovalRiskHigh),
		Actions:         actions,
	}
	return s.Create(req, now)
}

func (s *Service) Get(approvalID string) (model.ApprovalRecord, bool, error) {
	return s.repo.Get(approvalID)
}

func (s *Service) List() ([]model.ApprovalRecord, error) {
	return s.repo.List()
}

func (s *Service) Decide(approvalID string, approve bool, req model.ApprovalDecisionRequest, now time.Time) (rec model.ApprovalRecord, changed bool, conflict bool, notFound bool, err error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rec, ok, err := s.repo.Get(approvalID)
	if err != nil {
		return model.ApprovalRecord{}, false, false, false, err
	}
	if !ok {
		return model.ApprovalRecord{}, false, false, true, nil
	}
	target := model.ApprovalStatusRejected
	if approve {
		target = model.ApprovalStatusApproved
	}
	if rec.Status == model.ApprovalStatusPending && now.After(rec.ExpiresAt) {
		rec.Status = model.ApprovalStatusExpired
		rec.UpdatedAt = now
		if err := s.repo.Save(rec); err != nil {
			return model.ApprovalRecord{}, false, false, false, err
		}
		return rec, false, true, false, nil
	}
	if rec.Status == target {
		return rec, false, false, false, nil
	}
	if rec.Status != model.ApprovalStatusPending {
		return rec, false, true, false, nil
	}
	rec.Status = target
	rec.Decision = &model.ApprovalDecision{
		Actor:     req.Actor,
		Note:      req.Note,
		DecidedAt: now,
	}
	rec.UpdatedAt = now
	if err := s.repo.Save(rec); err != nil {
		return model.ApprovalRecord{}, false, false, false, err
	}
	_ = appendEvolution(s.workspace, "approval decided", []string{
		"approval_id=" + rec.ApprovalID,
		"status=" + string(rec.Status),
	}, now)
	return rec, true, false, false, nil
}

func (s *Service) PublishDecisionEvent(ctx context.Context, rec model.ApprovalRecord, now time.Time) error {
	if s.bus == nil {
		return nil
	}
	payloadType := ""
	switch rec.Status {
	case model.ApprovalStatusApproved:
		payloadType = "approval_granted"
	case model.ApprovalStatusRejected:
		payloadType = "approval_rejected"
	default:
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actionIDs := make([]string, 0, len(rec.Actions))
	for _, action := range rec.Actions {
		if action.ActionID == "" {
			continue
		}
		actionIDs = append(actionIDs, action.ActionID)
	}
	actor := &model.ApprovalActor{Type: "user", ID: "unknown"}
	note := ""
	if rec.Decision != nil {
		actor = &rec.Decision.Actor
		note = rec.Decision.Note
	}
	evt := model.InternalEvent{
		EventID:        fmt.Sprintf("evt_%d", now.UnixNano()),
		Source:         "approval",
		TenantID:       "local",
		Conversation:   model.Conversation{ConversationID: rec.ConversationID, ChannelType: "dm"},
		SessionKey:     rec.SessionKey,
		IdempotencyKey: fmt.Sprintf("approval:%s:%s", rec.ApprovalID, rec.Status),
		Timestamp:      now,
		Payload: model.EventPayload{
			Type:       payloadType,
			ApprovalID: rec.ApprovalID,
			Actor:      actor,
			Note:       note,
			Actions:    actionIDs,
		},
		ActiveSessionID: rec.SessionID,
	}
	return s.bus.PublishInbound(ctx, evt)
}

func (s *Service) ExecuteApproved(approvalID string, now time.Time) (string, []model.ApprovalActionResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rec, ok, err := s.repo.Get(approvalID)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, fmt.Errorf("approval not found")
	}
	if rec.Status != model.ApprovalStatusApproved {
		return "", nil, fmt.Errorf("approval status is %s", rec.Status)
	}

	results := make([]model.ApprovalActionResult, 0, len(rec.Actions))
	failed := 0
	for _, action := range rec.Actions {
		switch action.Type {
		case "Patch":
			patchPayload, err := decodePatchPayload(action.Payload)
			if err != nil {
				failed++
				results = append(results, model.ApprovalActionResult{
					ActionID: action.ActionID,
					OK:       false,
					Message:  fmt.Sprintf("decode patch payload failed: %v", err),
				})
				continue
			}
			patchResult, err := s.patcher.Apply(patchPayload, now)
			if err != nil {
				return "", nil, err
			}
			if !patchResult.OK {
				failed++
			}
			results = append(results, model.ApprovalActionResult{
				ActionID: action.ActionID,
				OK:       patchResult.OK,
				Message:  patchResult.Message,
			})
		default:
			failed++
			results = append(results, model.ApprovalActionResult{
				ActionID: action.ActionID,
				OK:       false,
				Message:  "unsupported action type",
			})
		}
	}

	rec.Results = results
	rec.UpdatedAt = now
	if err := s.repo.Save(rec); err != nil {
		return "", nil, err
	}

	evolutionLines := []string{
		"approval_id=" + rec.ApprovalID,
		"status=" + string(rec.Status),
	}
	for _, result := range results {
		evolutionLines = append(evolutionLines, fmt.Sprintf("action=%s ok=%t msg=%s", result.ActionID, result.OK, result.Message))
	}
	_ = appendEvolution(s.workspace, "approval executed", evolutionLines, now)

	if len(results) == 0 {
		return "审批已通过，无可执行动作。", results, nil
	}
	if failed == 0 {
		return "审批已通过，Patch 已应用。", results, nil
	}
	return "审批已通过，但部分 Patch 执行失败并已回滚。", results, nil
}

func parseRisk(raw string) model.ApprovalRisk {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low":
		return model.ApprovalRiskLow
	case "medium":
		return model.ApprovalRiskMedium
	case "high":
		return model.ApprovalRiskHigh
	default:
		return model.ApprovalRiskHigh
	}
}
