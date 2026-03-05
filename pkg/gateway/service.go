package gateway

import (
	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

// Service 聚合网关入站处理所需依赖，负责把 HTTP 请求转换为内部事件流程。
type Service struct {
	cfg            config.Config
	eventBus       *bus.MessageBus
	idempotency    *idempotency.Store
	sessions       *store.SessionStore
	events         *runtime.EventRepo
	adkRouter      adkSessionRouter
	tenantLimiter  *limiter
	sessionLimiter *limiter
}

// NewService 构造网关服务，并初始化租户级/会话级限流器。
func NewService(cfg config.Config, eventBus *bus.MessageBus, idem *idempotency.Store, sessions *store.SessionStore, events *runtime.EventRepo, adkRouter adkSessionRouter) *Service {
	return &Service{
		cfg:            cfg,
		eventBus:       eventBus,
		idempotency:    idem,
		sessions:       sessions,
		events:         events,
		adkRouter:      adkRouter,
		tenantLimiter:  newLimiter(cfg.RateLimitTenantRPS, cfg.RateLimitTenantBurst),
		sessionLimiter: newLimiter(cfg.RateLimitSessionRPS, cfg.RateLimitSessionBurst),
	}
}
