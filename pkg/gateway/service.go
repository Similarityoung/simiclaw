package gateway

import (
	"github.com/similarityyoung/simiclaw/pkg/config"
	runtime "github.com/similarityyoung/simiclaw/pkg/eventing"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

// Service 聚合网关入站处理所需依赖，负责把 HTTP 请求转换为内部事件流程。
type Service struct {
	cfg            config.Config
	idempotency    *idempotency.Store
	sessions       *store.SessionStore
	events         *runtime.EventRepo
	storeLoop      *store.StoreLoop
	adkRouter      adkSessionRouter
	tenantLimiter  *limiter
	sessionLimiter *limiter
}

// NewService 构造网关服务，并初始化租户级/会话级限流器。
func NewService(cfg config.Config, idem *idempotency.Store, sessions *store.SessionStore, events *runtime.EventRepo, storeLoop *store.StoreLoop, adkRouter adkSessionRouter) *Service {
	return &Service{
		cfg:            cfg,
		idempotency:    idem,
		sessions:       sessions,
		events:         events,
		storeLoop:      storeLoop,
		adkRouter:      adkRouter,
		tenantLimiter:  newLimiter(cfg.RateLimitTenantRPS, cfg.RateLimitTenantBurst),
		sessionLimiter: newLimiter(cfg.RateLimitSessionRPS, cfg.RateLimitSessionBurst),
	}
}
