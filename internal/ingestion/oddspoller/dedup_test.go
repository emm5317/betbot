package oddspoller

import "testing"

func TestShouldInsertSnapshot(t *testing.T) {
	snapshot := CanonicalOddsSnapshot{SnapshotHash: "after"}
	if !ShouldInsertSnapshot("before", snapshot) {
		t.Fatal("expected changed snapshot to insert")
	}
	if ShouldInsertSnapshot("after", snapshot) {
		t.Fatal("expected unchanged snapshot to skip")
	}
}
