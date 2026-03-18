package model

import (
	"time"

	pkgmodel "github.com/similarityyoung/simiclaw/pkg/model"
)

type NormalizedIngress struct {
	Source         string
	Conversation   pkgmodel.Conversation
	SessionKeyHint string
	IdempotencyKey string
	Timestamp      time.Time
	DMScope        string
	Payload        pkgmodel.EventPayload
	Metadata       map[string]string
}

type BindingContext struct {
	TenantID     string
	SessionKey   string
	SessionID    string
	Scope        string
	Conversation pkgmodel.Conversation
	Metadata     map[string]string
}

type RouteDecision struct {
	RunMode        pkgmodel.RunMode
	PayloadType    string
	SuppressOutput bool
	Metadata       map[string]string
}
