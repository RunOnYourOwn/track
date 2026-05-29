package db

import "testing"

// IDs minted in quick succession (same millisecond) must strictly increase, so
// they sort in creation order and can't collide.
func TestNewIDMonotonic(t *testing.T) {
	prev := NewID()
	for i := 0; i < 2000; i++ {
		id := NewID()
		if id <= prev {
			t.Fatalf("ULID not monotonic at i=%d: %q <= %q", i, id, prev)
		}
		prev = id
	}
}
