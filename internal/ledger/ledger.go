package ledger

import "torment/internal/db/sqlc"

// Service runs ledger operations against Postgres.
type Service struct {
	q sqlc.Querier
}

func New(q sqlc.Querier) *Service {
	return &Service{q: q}
}
