package ledger

import (
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestIDBytes(t *testing.T) {
	id := tb.ID()
	got, err := idFromBytes(idToBytes(id))
	if err != nil || got != id {
		t.Fatalf("round trip = %v, %v; want %v", got, err, id)
	}
	for _, n := range []int{0, 15, 17} {
		if _, err := idFromBytes(make([]byte, n)); err == nil {
			t.Errorf("idFromBytes(%d bytes) err = nil, want an error", n)
		}
	}
}
