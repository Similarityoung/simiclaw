package ingest

import (
	"github.com/similarityyoung/simiclaw/internal/ingest/port"
)

var ErrIdempotencyConflict = port.ErrIdempotencyConflict

type PersistRequest = port.PersistRequest
type PersistResult = port.PersistResult
type SessionScopeRecord = port.SessionScopeRecord
type Repository = port.Repository
type SessionReader = port.SessionReader
