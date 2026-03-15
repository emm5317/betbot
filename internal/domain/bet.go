package domain

// BetStatus represents the lifecycle state of a placed bet.
type BetStatus string

const (
	BetStatusPending BetStatus = "pending"
	BetStatusPlaced  BetStatus = "placed"
	BetStatusSettled BetStatus = "settled"
	BetStatusFailed  BetStatus = "failed"
	BetStatusVoided  BetStatus = "voided"
)

// SettlementResult represents the outcome of a settled bet.
type SettlementResult string

const (
	SettlementWin  SettlementResult = "win"
	SettlementLoss SettlementResult = "loss"
	SettlementPush SettlementResult = "push"
)
