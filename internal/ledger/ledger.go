package ledger

import "torment/internal/db"

// Service runs ledger operations against Postgres.
type Service struct {
	q db.Querier
}

func New(q db.Querier) *Service {
	return &Service{q: q}
}
