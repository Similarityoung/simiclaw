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
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

// ErrConflict 表示同一幂等键对应了不同 payload，属于语义冲突。
var ErrConflict = errors.New("idempotency payload hash mismatch")

// Store 管理 inbound/outbound 幂等账本，并负责账本文件持久化。
type Store struct {
	mu           sync.Mutex
	inbound      map[string]model.InboundLedgerRow
	outbound     map[string]model.OutboundLedgerRow
	inboundPath  string
	outboundPath string
}

// New 创建幂等账本存储并从磁盘恢复历史记录。
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

// load 读取 inbound/outbound JSONL 账本到内存映射。
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

// RegisterInbound 注册 inbound 幂等键；若已存在则返回已有记录或冲突。
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

// LookupInbound 查询 inbound 幂等记录是否存在。
func (s *Store) LookupInbound(idempotencyKey string) (model.InboundLedgerRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.inbound[idempotencyKey]
	return row, ok
}

// RegisterOutbound 注册 outbound 幂等键，避免同一事件重复对外发送。
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

// DeleteInbound 删除指定 inbound 幂等记录并重写 inbound 账本文件。
func (s *Store) DeleteInbound(idempotencyKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.inbound[idempotencyKey]; !ok {
		return nil
	}
	delete(s.inbound, idempotencyKey)
	return s.rewriteInboundLocked()
}

// rewriteInboundLocked 将内存中的 inbound 映射重建为稳定有序的 JSONL 文件。
func (s *Store) rewriteInboundLocked() error {
	rows := make([]model.InboundLedgerRow, 0, len(s.inbound))
	for _, row := range s.inbound {
		rows = append(rows, row)
	}
	// 按时间再按幂等键排序，保证重写结果可预测、便于排查差异。
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
