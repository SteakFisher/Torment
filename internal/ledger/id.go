package ledger

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// idToBytes returns id as 16 bytes in TigerBeetle's little-endian order, the form stored in Postgres.
func idToBytes(id tb.Uint128) []byte {
	b := id.Bytes()
	return b[:]
}

// idFromBytes is the inverse of idToBytes. It returns an error when b is not 16 bytes long.
func idFromBytes(b []byte) (tb.Uint128, error) {
	if len(b) != 16 {
		return tb.Uint128{}, fmt.Errorf("tigerbeetle id must be 16 bytes, got %d", len(b))
	}
	return tb.BytesToUint128([16]byte(b)), nil
}
