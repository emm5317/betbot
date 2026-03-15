package execution

import (
	"fmt"
	"time"
)

// GenerateIdempotencyKey produces a deterministic key for exactly-once placement.
// Formula: game_id:market:book:timestamp_bucket (per CLAUDE.md invariant #1).
// The timestamp is floored to the minute of the recommendation's GeneratedAt.
func GenerateIdempotencyKey(gameID int64, marketKey, bookKey string, generatedAt time.Time) string {
	bucket := generatedAt.UTC().Truncate(time.Minute).Unix()
	return fmt.Sprintf("%d:%s:%s:%d", gameID, marketKey, bookKey, bucket)
}
