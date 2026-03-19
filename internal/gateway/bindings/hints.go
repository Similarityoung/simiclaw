package bindings

const (
	HintPayloadTypeOverride = "payload_type_override"
	HintScopeSource         = "scope_source"

	ScopeSourceNewSession = "new_session"
	ScopeSourceSessionKey = "session_key_hint"
	ScopeSourceIngress    = "ingress"
	ScopeSourceStored     = "stored"
	ScopeSourceDefault    = "default"
)

func WithHint(meta map[string]string, key, value string) map[string]string {
	if value == "" {
		return meta
	}
	if meta == nil {
		meta = make(map[string]string, 1)
	}
	meta[key] = value
	return meta
}

func PayloadTypeOverride(meta map[string]string) string {
	if meta == nil {
		return ""
	}
	return meta[HintPayloadTypeOverride]
}
