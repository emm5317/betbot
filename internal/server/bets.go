package server

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func (a *App) handleBetsPage(c fiber.Ctx) error {
	sportFilter := c.Query("sport")
	statusFilter := c.Query("status")

	pnl, err := a.queries.GetBetPnLSummary(c.Context(), sportFilter)
	if err != nil {
		return fmt.Errorf("get bet pnl summary: %w", err)
	}

	bets, err := a.queries.ListBetsWithFilters(c.Context(), store.ListBetsWithFiltersParams{
		Sport:        sportFilter,
		StatusFilter: statusFilter,
		RowLimit:     100,
	})
	if err != nil {
		return fmt.Errorf("list bets: %w", err)
	}

	betRows := make([]map[string]any, 0, len(bets))
	for _, b := range bets {
		row := map[string]any{
			"ID":              b.ID,
			"Sport":           b.Sport,
			"HomeTeam":        b.HomeTeam,
			"AwayTeam":        b.AwayTeam,
			"CommenceTime":    formatTimestamp(b.CommenceTime, "—"),
			"Side":            b.RecommendedSide,
			"BookKey":         b.BookKey,
			"AmericanOdds":    formatAmericanOdds(b.AmericanOdds),
			"StakeDollars":    formatCents(b.StakeCents),
			"Status":          string(b.Status),
			"SettlementResult": "",
			"PayoutDollars":   "—",
			"PnLDollars":      "—",
			"PlacedAt":        formatTimestamp(b.PlacedAt, "—"),
		}
		if b.SettlementResult != nil {
			row["SettlementResult"] = *b.SettlementResult
		}
		if b.PayoutCents != nil {
			row["PayoutDollars"] = formatCents(*b.PayoutCents)
			row["PnLDollars"] = formatCentsSigned(*b.PayoutCents - b.StakeCents)
		}
		betRows = append(betRows, row)
	}

	roi := 0.0
	if pnl.TotalStakedCents > 0 {
		settledStaked := pnl.TotalReturnedCents - int64(pnl.NetPnlCents) // settled stake = returned - pnl
		if settledStaked > 0 {
			roi = float64(pnl.NetPnlCents) / float64(settledStaked) * 100
		}
	}

	_, overallStatus := a.pipelineView(c.Context(), sportFilterSelection{})

	view := map[string]any{
		"Title":              "Bet Ledger",
		"ActiveNav":          "bets",
		"OverallStatus":      overallStatus,
		"Environment":        a.cfg.Env,
		"Bets":               betRows,
		"TotalBets":          pnl.TotalBets,
		"OpenBets":           pnl.OpenBets,
		"SettledBets":        pnl.SettledBets,
		"VoidedBets":         pnl.VoidedBets,
		"TotalStaked":        formatCents(pnl.TotalStakedCents),
		"TotalReturned":      formatCents(pnl.TotalReturnedCents),
		"NetPnL":             formatCentsSigned(int64(pnl.NetPnlCents)),
		"NetPnLPositive":     pnl.NetPnlCents >= 0,
		"ROI":                fmt.Sprintf("%.1f%%", roi),
		"SportFilter":        sportFilter,
		"StatusFilter":       statusFilter,
		"SelectedSportQuery": "",
	}

	return c.Render("pages/bets", view, "layouts/base")
}

func (a *App) handleBetsNewPage(c fiber.Ctx) error {
	_, overallStatus := a.pipelineView(c.Context(), sportFilterSelection{})

	view := map[string]any{
		"Title":              "Record a Bet",
		"ActiveNav":          "bets",
		"OverallStatus":      overallStatus,
		"Environment":        a.cfg.Env,
		"SelectedSportQuery": "",
	}

	// If snapshot_id provided, pre-fill from recommendation
	if snapshotIDStr := c.Query("snapshot_id"); snapshotIDStr != "" {
		snapshotID, err := strconv.ParseInt(snapshotIDStr, 10, 64)
		if err == nil {
			snapshot, err := a.queries.GetRecommendationSnapshotByID(c.Context(), snapshotID)
			if err == nil {
				view["SnapshotID"] = snapshot.ID
				view["PrefilledSport"] = snapshot.Sport
				view["PrefilledGameID"] = snapshot.GameID
				view["PrefilledSide"] = snapshot.RecommendedSide
				view["PrefilledBook"] = snapshot.BestBook
				view["PrefilledOdds"] = snapshot.BestAmericanOdds
				view["PrefilledStakeDollars"] = fmt.Sprintf("%.2f", float64(snapshot.SuggestedStakeCents)/100.0)
				view["PrefilledEdge"] = fmt.Sprintf("%.1f%%", snapshot.Edge*100)
				view["PrefilledModelProb"] = fmt.Sprintf("%.1f%%", snapshot.ModelProbability*100)

				// Also load the game info
				game, err := a.queries.GetGameByID(c.Context(), snapshot.GameID)
				if err == nil {
					view["PrefilledGameLabel"] = fmt.Sprintf("%s vs %s", game.HomeTeam, game.AwayTeam)
				}
			}
		}
	}

	// Load upcoming games for the dropdown
	games, err := a.queries.ListUpcomingGames(c.Context(), 100)
	if err != nil {
		return fmt.Errorf("list upcoming games: %w", err)
	}
	gameOptions := make([]map[string]any, 0, len(games))
	for _, g := range games {
		gameOptions = append(gameOptions, map[string]any{
			"ID":           g.ID,
			"Sport":        g.Sport,
			"HomeTeam":     g.HomeTeam,
			"AwayTeam":     g.AwayTeam,
			"CommenceTime": formatTimestamp(g.CommenceTime, "TBD"),
			"Label":        fmt.Sprintf("[%s] %s vs %s", g.Sport, g.HomeTeam, g.AwayTeam),
		})
	}
	view["Games"] = gameOptions

	return c.Render("pages/bets_new", view, "layouts/base")
}

func (a *App) handleBetsCreate(c fiber.Ctx) error {
	// Parse form values
	gameIDStr := strings.TrimSpace(c.FormValue("game_id"))
	sport := strings.TrimSpace(c.FormValue("sport"))
	side := strings.TrimSpace(c.FormValue("side"))
	bookKey := strings.TrimSpace(c.FormValue("book_key"))
	oddsStr := strings.TrimSpace(c.FormValue("american_odds"))
	stakeStr := strings.TrimSpace(c.FormValue("stake_dollars"))
	notes := strings.TrimSpace(c.FormValue("notes"))
	snapshotIDStr := strings.TrimSpace(c.FormValue("snapshot_id"))

	// Validate required fields
	if gameIDStr == "" || sport == "" || side == "" || bookKey == "" || oddsStr == "" || stakeStr == "" {
		return c.Status(fiber.StatusBadRequest).SendString("All required fields must be filled")
	}

	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid game ID")
	}

	odds, err := strconv.ParseInt(oddsStr, 10, 32)
	if err != nil || (odds > -100 && odds < 100) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid American odds (must be <= -100 or >= 100)")
	}

	stakeDollars, err := strconv.ParseFloat(stakeStr, 64)
	if err != nil || stakeDollars <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid stake amount")
	}
	stakeCents := int64(math.Round(stakeDollars * 100))

	var snapshotID *int64
	if snapshotIDStr != "" {
		sid, err := strconv.ParseInt(snapshotIDStr, 10, 64)
		if err == nil {
			snapshotID = &sid
		}
	}

	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}

	idempotencyKey := uuid.NewString()

	// Use a transaction: insert bet + reserve stake
	tx, err := a.pgxPool.Begin(c.Context())
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(c.Context()) //nolint:errcheck

	txQueries := store.New(tx)

	bet, err := txQueries.InsertManualBet(c.Context(), store.InsertManualBetParams{
		IdempotencyKey:  idempotencyKey,
		SnapshotID:      snapshotID,
		GameID:          gameID,
		Sport:           sport,
		MarketKey:       "h2h",
		RecommendedSide: side,
		BookKey:         bookKey,
		AmericanOdds:    int32(odds),
		StakeCents:      stakeCents,
		UserNotes:       notesPtr,
	})
	if err != nil {
		return fmt.Errorf("insert manual bet: %w", err)
	}

	// Reserve stake in bankroll ledger
	_, err = txQueries.InsertBankrollEntry(c.Context(), store.InsertBankrollEntryParams{
		EntryType:     "bet_stake_reserved",
		AmountCents:   -stakeCents,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   idempotencyKey,
		Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d}`, bet.ID)),
	})
	if err != nil {
		return fmt.Errorf("reserve stake: %w", err)
	}

	if err := tx.Commit(c.Context()); err != nil {
		return fmt.Errorf("commit bet tx: %w", err)
	}

	return c.Redirect().To("/bets")
}

func (a *App) handleBetsSettle(c fiber.Ctx) error {
	betIDStr := c.Params("id")
	betID, err := strconv.ParseInt(betIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid bet ID")
	}

	result := c.FormValue("result")
	if result != "win" && result != "loss" && result != "push" {
		return c.Status(fiber.StatusBadRequest).SendString("Result must be win, loss, or push")
	}

	bet, err := a.queries.GetBetByID(c.Context(), betID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Bet not found")
	}
	if bet.Status != store.BetStatusPlaced {
		return c.Status(fiber.StatusBadRequest).SendString("Bet is not in placed status")
	}

	// Calculate payout
	var payoutCents int64
	switch result {
	case "win":
		payoutCents = bet.StakeCents + calculateWinnings(bet.StakeCents, int(bet.AmericanOdds))
	case "push":
		payoutCents = bet.StakeCents
	case "loss":
		payoutCents = 0
	}

	// Transaction: update bet + write ledger entry
	tx, err := a.pgxPool.Begin(c.Context())
	if err != nil {
		return fmt.Errorf("begin settle tx: %w", err)
	}
	defer tx.Rollback(c.Context()) //nolint:errcheck

	txQueries := store.New(tx)

	if err := txQueries.UpdateBetSettled(c.Context(), store.UpdateBetSettledParams{
		ID:               betID,
		SettlementResult: &result,
		PayoutCents:      &payoutCents,
	}); err != nil {
		return fmt.Errorf("update bet settled: %w", err)
	}

	// Write ledger entry
	ledgerAmt := settlementLedgerAmount(result, bet.StakeCents, payoutCents)
	entryType := "bet_settlement_loss"
	switch result {
	case "win":
		entryType = "bet_settlement_win"
	case "push":
		entryType = "bet_settlement_push"
	}

	_, err = txQueries.InsertBankrollEntry(c.Context(), store.InsertBankrollEntryParams{
		EntryType:     entryType,
		AmountCents:   ledgerAmt,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   bet.IdempotencyKey,
		Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d,"result":"%s"}`, betID, result)),
	})
	if err != nil {
		return fmt.Errorf("write settlement ledger: %w", err)
	}

	if err := tx.Commit(c.Context()); err != nil {
		return fmt.Errorf("commit settle tx: %w", err)
	}

	return c.Redirect().To("/bets")
}

func (a *App) handleBetsVoid(c fiber.Ctx) error {
	betIDStr := c.Params("id")
	betID, err := strconv.ParseInt(betIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid bet ID")
	}

	bet, err := a.queries.GetBetByID(c.Context(), betID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Bet not found")
	}
	if bet.Status != store.BetStatusPlaced {
		return c.Status(fiber.StatusBadRequest).SendString("Bet is not in placed status")
	}

	tx, err := a.pgxPool.Begin(c.Context())
	if err != nil {
		return fmt.Errorf("begin void tx: %w", err)
	}
	defer tx.Rollback(c.Context()) //nolint:errcheck

	txQueries := store.New(tx)

	if err := txQueries.VoidBet(c.Context(), betID); err != nil {
		return fmt.Errorf("void bet: %w", err)
	}

	// Release stake back to bankroll
	_, err = txQueries.InsertBankrollEntry(c.Context(), store.InsertBankrollEntryParams{
		EntryType:     "bet_stake_released",
		AmountCents:   bet.StakeCents,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   bet.IdempotencyKey,
		Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d,"reason":"voided"}`, betID)),
	})
	if err != nil {
		return fmt.Errorf("release voided stake: %w", err)
	}

	if err := tx.Commit(c.Context()); err != nil {
		return fmt.Errorf("commit void tx: %w", err)
	}

	return c.Redirect().To("/bets")
}

// calculateWinnings computes the profit from a winning bet given American odds.
func calculateWinnings(stakeCents int64, americanOdds int) int64 {
	var multiplier float64
	if americanOdds > 0 {
		multiplier = float64(americanOdds) / 100.0
	} else {
		multiplier = 100.0 / math.Abs(float64(americanOdds))
	}
	return int64(math.Round(float64(stakeCents) * multiplier))
}

// settlementLedgerAmount returns the amount to credit back to bankroll.
// Win: +payout (stake + profit). Loss: 0. Push: +stake refund.
func settlementLedgerAmount(result string, stakeCents, payoutCents int64) int64 {
	switch result {
	case "win":
		return payoutCents
	case "push":
		return stakeCents
	default:
		return 0
	}
}

func formatCents(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

func formatCentsSigned(cents int64) string {
	dollars := float64(cents) / 100.0
	if cents >= 0 {
		return fmt.Sprintf("+$%.2f", dollars)
	}
	return fmt.Sprintf("-$%.2f", -dollars)
}

func formatAmericanOdds(odds int32) string {
	if odds > 0 {
		return fmt.Sprintf("+%d", odds)
	}
	return fmt.Sprintf("%d", odds)
}

// formatTimeSince formats a timestamp relative to now (for use in templates).
func formatTimeSince(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
