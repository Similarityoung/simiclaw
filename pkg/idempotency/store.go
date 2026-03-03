package idempotency

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

var ErrConflict = errors.New("idempotency payload hash mismatch")

type Store struct {
	mu           sync.Mutex
	inbound      map[string]model.InboundLedgerRow
	outbound     map[string]model.OutboundLedgerRow
	inboundPath  string
	outboundPath string
}

func New(workspace string) (*Store, error) {
	s := &Store{
		inbound:      map[string]model.InboundLedgerRow{},
		outbound:     map[string]model.OutboundLedgerRow{},
		inboundPath:  filepath.Join(workspace, "runtime", "idempotency", "inbound_keys.jsonl"),
		outboundPath: filepath.Join(workspace, "runtime", "idempotency", "outbound_keys.jsonl"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	inboundRows, err := store.ReadJSONLines[model.InboundLedgerRow](s.inboundPath)
	if err != nil {
		return err
	}
	for _, row := range inboundRows {
		s.inbound[row.IdempotencyKey] = row
	}

	outboundRows, err := store.ReadJSONLines[model.OutboundLedgerRow](s.outboundPath)
	if err != nil {
		return err
	}
	for _, row := range outboundRows {
		s.outbound[row.OutboundIdempotencyKey] = row
	}
	return nil
}

func (s *Store) RegisterInbound(idempotencyKey, payloadHash, eventID, sessionKey, activeSessionID string, receivedAt time.Time) (row model.InboundLedgerRow, duplicated bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.inbound[idempotencyKey]; ok {
		if existing.PayloadHash != payloadHash {
			return existing, false, ErrConflict
		}
		return existing, true, nil
	}

	row = model.InboundLedgerRow{
		IdempotencyKey:  idempotencyKey,
		EventID:         eventID,
		PayloadHash:     payloadHash,
		ReceivedAt:      receivedAt,
		SessionKey:      sessionKey,
		ActiveSessionID: activeSessionID,
	}
	if err := store.AppendJSONL(s.inboundPath, row); err != nil {
		return model.InboundLedgerRow{}, false, err
	}
	s.inbound[idempotencyKey] = row
	return row, false, nil
}

func (s *Store) LookupInbound(idempotencyKey string) (model.InboundLedgerRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.inbound[idempotencyKey]
	return row, ok
}

func (s *Store) RegisterOutbound(outboundIdempotencyKey, outboxID string, now time.Time) (row model.OutboundLedgerRow, duplicated bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.outbound[outboundIdempotencyKey]; ok {
		return existing, true, nil
	}
	row = model.OutboundLedgerRow{
		OutboundIdempotencyKey: outboundIdempotencyKey,
		OutboxID:               outboxID,
		CreatedAt:              now,
	}
	if err := store.AppendJSONL(s.outboundPath, row); err != nil {
		return model.OutboundLedgerRow{}, false, err
	}
	s.outbound[outboundIdempotencyKey] = row
	return row, false, nil
}

func (s *Store) DeleteInbound(idempotencyKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.inbound[idempotencyKey]; !ok {
		return nil
	}
	delete(s.inbound, idempotencyKey)
	return s.rewriteInboundLocked()
}

func (s *Store) rewriteInboundLocked() error {
	rows := make([]model.InboundLedgerRow, 0, len(s.inbound))
	for _, row := range s.inbound {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ReceivedAt.Equal(rows[j].ReceivedAt) {
			return rows[i].IdempotencyKey < rows[j].IdempotencyKey
		}
		return rows[i].ReceivedAt.Before(rows[j].ReceivedAt)
	})

	var buf bytes.Buffer
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := buf.Write(b); err != nil {
			return err
		}
		if err := buf.WriteByte('\n'); err != nil {
			return err
		}
	}
	return store.AtomicWriteFile(s.inboundPath, buf.Bytes(), 0o644)
}
