package execution

import (
	"testing"
	"time"
)

func TestGenerateIdempotencyKeyDeterministic(t *testing.T) {
	ts := time.Date(2025, 3, 15, 14, 30, 45, 0, time.UTC)

	key1 := GenerateIdempotencyKey(123, "h2h", "fanduel", ts)
	key2 := GenerateIdempotencyKey(123, "h2h", "fanduel", ts)

	if key1 != key2 {
		t.Fatalf("idempotency keys must be deterministic: %q != %q", key1, key2)
	}
}

func TestGenerateIdempotencyKeyFloorToMinute(t *testing.T) {
	ts1 := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	ts2 := time.Date(2025, 3, 15, 14, 30, 59, 999999999, time.UTC)

	key1 := GenerateIdempotencyKey(42, "spread", "draftkings", ts1)
	key2 := GenerateIdempotencyKey(42, "spread", "draftkings", ts2)

	if key1 != key2 {
		t.Fatalf("keys within same minute must match: %q != %q", key1, key2)
	}
}

func TestGenerateIdempotencyKeyDifferentMinutesDiffer(t *testing.T) {
	ts1 := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	ts2 := time.Date(2025, 3, 15, 14, 31, 0, 0, time.UTC)

	key1 := GenerateIdempotencyKey(42, "spread", "draftkings", ts1)
	key2 := GenerateIdempotencyKey(42, "spread", "draftkings", ts2)

	if key1 == key2 {
		t.Fatal("keys in different minutes must differ")
	}
}

func TestGenerateIdempotencyKeyDifferentInputsDiffer(t *testing.T) {
	ts := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)

	keyA := GenerateIdempotencyKey(100, "h2h", "fanduel", ts)
	keyB := GenerateIdempotencyKey(200, "h2h", "fanduel", ts)
	keyC := GenerateIdempotencyKey(100, "spread", "fanduel", ts)
	keyD := GenerateIdempotencyKey(100, "h2h", "draftkings", ts)

	keys := []string{keyA, keyB, keyC, keyD}
	seen := make(map[string]struct{})
	for _, k := range keys {
		if _, exists := seen[k]; exists {
			t.Fatalf("duplicate key found: %q", k)
		}
		seen[k] = struct{}{}
	}
}
