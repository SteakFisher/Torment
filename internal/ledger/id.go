package ledger

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"slices"

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

// parseID parses s as a UUID in the hyphenated 8-4-4-4-12 hex form, in any case, and returns its 16 bytes
// read big-endian as a TigerBeetle id. It reports false for any other form and for the nil and max
// UUIDs, which TigerBeetle rejects as ids.
func parseID(s string) (tb.Uint128, bool) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return tb.Uint128{}, false
	}
	var b [16]byte
	if _, err := hex.Decode(b[:], []byte(s[:8]+s[9:13]+s[14:18]+s[19:23]+s[24:])); err != nil {
		return tb.Uint128{}, false
	}
	// tb.Uint128's byte form is little-endian.
	slices.Reverse(b[:])
	id := tb.BytesToUint128(b)
	if id == (tb.Uint128{}) || id == maxUint128 {
		return tb.Uint128{}, false
	}
	return id, true
}

// FormatID returns id as a lowercase hyphenated UUID whose 16 bytes are id big-endian. It is the inverse
// of parseID.
func FormatID(id tb.Uint128) string {
	b := id.Bytes()
	slices.Reverse(b[:])
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
