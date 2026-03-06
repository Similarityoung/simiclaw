package gateway

import (
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type Service struct {
	cfg            config.Config
	db             *store.DB
	eventLoop      *runtime.EventLoop
	tenantLimiter  *limiter
	sessionLimiter *limiter
}

func NewService(cfg config.Config, db *store.DB, eventLoop *runtime.EventLoop) *Service {
	return &Service{
		cfg:            cfg,
		db:             db,
		eventLoop:      eventLoop,
		tenantLimiter:  newLimiter(cfg.RateLimitTenantRPS, cfg.RateLimitTenantBurst),
		sessionLimiter: newLimiter(cfg.RateLimitSessionRPS, cfg.RateLimitSessionBurst),
	}
}
