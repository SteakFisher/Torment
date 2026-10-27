package ledger

import (
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// Service runs ledger operations against Postgres and TigerBeetle.
type Service struct {
	q  sqlc.Querier
	tb tbClient
}

func New(q sqlc.Querier, tb tbClient) *Service {
	return &Service{q: q, tb: tb}
}

// tbClient is the subset of the TigerBeetle client the Service calls.
type tbClient interface {
	CreateAccounts(accounts []tb.Account) ([]tb.CreateAccountResult, error)
	LookupAccounts(ids []tb.Uint128) ([]tb.Account, error)
}

// Fails to compile when tb.Client no longer satisfies tbClient.
var _ tbClient = tb.Client(nil)
