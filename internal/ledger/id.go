package ledger

import (
	"fmt"
	"math/big"

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

// parseUint128 parses s as a base-10 integer. It reports false unless the value is greater than zero
// and fits in 128 bits.
func parseUint128(s string) (tb.Uint128, bool) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() <= 0 || n.BitLen() > 128 {
		return tb.Uint128{}, false
	}
	return tb.BigIntToUint128(n), true
}
