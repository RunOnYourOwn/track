package db

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Monotonic entropy so IDs created within the same millisecond strictly increase
// (sortable in creation order, and no same-ms collisions). ulid.Monotonic isn't
// safe for concurrent use, so guard it with a mutex.
var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

func NewID() string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
