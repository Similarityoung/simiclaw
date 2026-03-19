package queries

import "github.com/similarityyoung/simiclaw/internal/store"

type Repository struct {
	db *store.DB
}

func NewRepository(db *store.DB) *Repository {
	if db == nil {
		return nil
	}
	return &Repository{db: db}
}
