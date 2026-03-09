package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const DefaultScope = "default"

func NormalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return DefaultScope
	}
	return scope
}

func IsNewSessionCommand(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) != 1 {
		return false
	}
	token := fields[0]
	return token == "/new" || strings.HasPrefix(token, "/new@")
}

func NewScopeFromID(idempotencyKey string) string {
	sum := sha256.Sum256([]byte("new_session:" + strings.TrimSpace(idempotencyKey)))
	return "scope_" + hex.EncodeToString(sum[:8])
}

func ScopeFromRequest(req model.IngestRequest) string {
	return NormalizeScope(req.DMScope)
}
