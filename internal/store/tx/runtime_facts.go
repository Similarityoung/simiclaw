package tx

import (
	"github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
)

type RuntimeRepository struct {
	db    *store.DB
	query *storequeries.Repository
}

func NewRuntimeRepository(db *store.DB) *RuntimeRepository {
	if db == nil {
		return nil
	}
	return &RuntimeRepository{
		db:    db,
		query: storequeries.NewRepository(db),
	}
}
