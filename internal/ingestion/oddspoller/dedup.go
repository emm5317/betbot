package oddspoller

func ShouldInsertSnapshot(previousHash string, snapshot CanonicalOddsSnapshot) bool {
	return previousHash != snapshot.SnapshotHash
}
