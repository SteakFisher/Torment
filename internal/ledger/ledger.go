package ledger

import "torment/internal/db/sqlc"

// Service runs ledger operations against Postgres and TigerBeetle.
type Service struct {
	q  sqlc.Querier
	tb tbClient
}

func New(q sqlc.Querier, tb tbClient) *Service {
	return &Service{q: q, tb: tb}
}
