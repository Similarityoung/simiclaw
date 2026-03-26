package tx

import "github.com/similarityyoung/simiclaw/internal/store"

type RuntimeRepository struct {
	db *store.DB
}

func NewRuntimeRepository(db *store.DB) *RuntimeRepository {
	if db == nil {
		return nil
	}
	return &RuntimeRepository{db: db}
}
