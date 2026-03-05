package outbound

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

type Sender interface {
	Send(ctx context.Context, msg model.OutboxMessage) error
}

type Hub struct {
	workspace string
	sender    Sender
	ledger    *idempotency.Store
}

func NewHub(workspace string, sender Sender, ledger *idempotency.Store) *Hub {
	return &Hub{workspace: workspace, sender: sender, ledger: ledger}
}

type SendResult struct {
	DeliveryStatus model.DeliveryStatus
	DeliveryDetail model.DeliveryDetail
	OutboxID       string
	Err            error
}

func (h *Hub) Send(ctx context.Context, eventID, sessionKey, body string) SendResult {
	start := time.Now()
	now := start.UTC()
	outboxID := fmt.Sprintf("out_%d", now.UnixNano())
	baseLogger := logging.L("outbound").With(
		logging.String("event_id", eventID),
		logging.String("session_key", sessionKey),
	)

	outKey := fmt.Sprintf("out:%s", eventID)
	row, duplicated, err := h.ledger.RegisterOutbound(outKey, outboxID, now)
	if err != nil {
		baseLogger.Error("outbound.send_failed_spooled",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return SendResult{DeliveryStatus: model.DeliveryStatusFailed, DeliveryDetail: model.DeliveryDetailSpooled, Err: err}
	}
	if duplicated {
		outboxID = row.OutboxID
	}
	logger := baseLogger.With(logging.String("outbox_id", outboxID))

	msg := model.OutboxMessage{
		OutboxID:               outboxID,
		OutboundIdempotencyKey: outKey,
		EventID:                eventID,
		SessionKey:             sessionKey,
		Body:                   body,
		CreatedAt:              now,
	}

	if err := h.sender.Send(ctx, msg); err != nil {
		msg.Attempts = 1
		msg.LastError = err.Error()
		spoolPath := filepath.Join(h.workspace, "runtime", "outbound_spool", outboxID+".json")
		if writeErr := store.AtomicWriteJSON(spoolPath, msg, 0o644); writeErr != nil {
			logger.Error("outbound.send_failed_spooled",
				logging.String("status", "failed"),
				logging.String("delivery_detail", string(model.DeliveryDetailSpooled)),
				logging.String("error_code", model.ErrorCodeInternal),
				logging.Error(err),
				logging.NamedError("spool_error", writeErr),
				logging.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
			return SendResult{DeliveryStatus: model.DeliveryStatusFailed, DeliveryDetail: model.DeliveryDetailSpooled, OutboxID: outboxID, Err: fmt.Errorf("send failed: %v; spool failed: %w", err, writeErr)}
		}
		logger.Warn("outbound.send_failed_spooled",
			logging.String("status", "failed"),
			logging.String("delivery_detail", string(model.DeliveryDetailSpooled)),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return SendResult{DeliveryStatus: model.DeliveryStatusFailed, DeliveryDetail: model.DeliveryDetailSpooled, OutboxID: outboxID, Err: err}
	}
	logger.Info("outbound.send_succeeded",
		logging.String("status", string(model.DeliveryStatusSent)),
		logging.String("delivery_detail", string(model.DeliveryDetailDirect)),
		logging.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
	return SendResult{DeliveryStatus: model.DeliveryStatusSent, DeliveryDetail: model.DeliveryDetailDirect, OutboxID: ""}
}

type StdoutSender struct{}

func (s StdoutSender) Send(_ context.Context, msg model.OutboxMessage) error {
	if strings.Contains(msg.Body, "[fail_outbound]") {
		return fmt.Errorf("simulated outbound failure")
	}
	return nil
}
