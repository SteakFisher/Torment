package ledger

import (
	"bytes"
	"math/big"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// maxUint128 is 2^128-1, which TigerBeetle rejects as an id.
var maxUint128 = tb.BytesToUint128([16]byte(bytes.Repeat([]byte{0xff}, 16)))

// parseUint128 parses s as a base-10 integer. It reports false unless the value is greater than zero
// and fits in 128 bits.
func parseUint128(s string) (tb.Uint128, bool) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() <= 0 || n.BitLen() > 128 {
		return tb.Uint128{}, false
	}
	return tb.BigIntToUint128(n), true
}
