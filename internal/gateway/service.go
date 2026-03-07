package gateway

import (
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Service struct {
	cfg            config.Config
	db             *store.DB
	eventLoop      *runtime.EventLoop
	tenantLimiter  *limiter
	sessionLimiter *limiter
}

type AcceptedIngest struct {
	Response   model.IngestResponse
	Result     store.IngestResult
	StatusCode int
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
