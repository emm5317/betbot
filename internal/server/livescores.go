package server

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"betbot/internal/livescores"

	"github.com/gofiber/fiber/v3"
)

func (a *App) handlePartialLiveScores(c fiber.Ctx) error {
	snapshot := a.scoreCache.Latest()

	data := map[string]any{
		"Games":     []map[string]any{},
		"FetchedAt": "",
		"HasGames":  false,
	}

	if snapshot == nil {
		return c.Render("partials/live_scores_block", data)
	}

	// Load open bets for overlay matching
	openBets, _ := a.queries.ListOpenBetsWithGame(c.Context())

	// Build bet lookup: key = "lowercase_home|lowercase_away"
	type betInfo struct {
		ID     int64
		Side   string
		Market string
		Stake  string
		Odds   string
	}
	betsByMatchup := make(map[string][]betInfo)
	for _, b := range openBets {
		if b.GameSport != "NHL" {
			continue
		}
		key := strings.ToLower(b.HomeTeam) + "|" + strings.ToLower(b.AwayTeam)
		betsByMatchup[key] = append(betsByMatchup[key], betInfo{
			ID:     b.ID,
			Side:   b.RecommendedSide,
			Market: b.MarketKey,
			Stake:  fmt.Sprintf("$%.2f", float64(b.StakeCents)/100),
			Odds:   formatOdds(int(b.AmericanOdds)),
		})
	}

	games := make([]livescores.LiveGame, len(snapshot.Games))
	copy(games, snapshot.Games)

	// Attach bet overlays
	for i := range games {
		key := strings.ToLower(games[i].HomeName) + "|" + strings.ToLower(games[i].AwayName)
		if bets, ok := betsByMatchup[key]; ok {
			for _, b := range bets {
				isWinning := evaluateBetStatus(b.Side, b.Market, games[i])
				games[i].Bets = append(games[i].Bets, livescores.LiveGameBet{
					BetID:     b.ID,
					Side:      b.Side,
					Market:    b.Market,
					Stake:     b.Stake,
					Odds:      b.Odds,
					IsWinning: isWinning,
				})
			}
		}
	}

	// Sort: bet games first (live > other), then live, then upcoming, then final
	sort.SliceStable(games, func(i, j int) bool {
		return gameSortKey(games[i]) < gameSortKey(games[j])
	})

	// Build template view models
	gameViews := make([]map[string]any, 0, len(games))
	for _, g := range games {
		gv := map[string]any{
			"NHLID":          g.NHLID,
			"GameState":      g.GameState,
			"IsLive":         g.IsLive(),
			"IsComplete":     g.IsComplete(),
			"Period":         g.Period,
			"PeriodLabel":    g.PeriodLabel,
			"Clock":          g.Clock,
			"InIntermission": g.InIntermission,
			"HomeAbbrev":     g.HomeAbbrev,
			"HomeScore":      g.HomeScore,
			"HomeSOG":        g.HomeSOG,
			"HomeRecord":     g.HomeRecord,
			"AwayAbbrev":     g.AwayAbbrev,
			"AwayScore":      g.AwayScore,
			"AwaySOG":        g.AwaySOG,
			"AwayRecord":     g.AwayRecord,
			"StartTimeUTC":   g.StartTimeUTC.Format(time.RFC3339),
			"HasBets":        g.HasBets(),
			"Bets":           mapBetViews(g.Bets),
			"TotalGoals":     g.TotalGoals(),
			"StatusLabel":    statusLabel(g),
		}
		gameViews = append(gameViews, gv)
	}

	data["Games"] = gameViews
	data["FetchedAt"] = snapshot.FetchedAt.Format(time.RFC3339)
	data["HasGames"] = len(gameViews) > 0

	return c.Render("partials/live_scores_block", data)
}

func evaluateBetStatus(side, market string, g livescores.LiveGame) bool {
	if !g.IsLive() && !g.IsComplete() {
		return false
	}
	switch market {
	case "h2h":
		if side == "home" {
			return g.HomeScore > g.AwayScore
		}
		return g.AwayScore > g.HomeScore
	case "totals":
		// For totals, "under" is winning if total is currently low.
		// We can't fully evaluate without the line, so just return false.
		return false
	}
	return false
}

func mapBetViews(bets []livescores.LiveGameBet) []map[string]any {
	views := make([]map[string]any, 0, len(bets))
	for _, b := range bets {
		sideLabel := "Away"
		if b.Side == "home" {
			sideLabel = "Home"
		}
		marketLabel := "ML"
		if b.Market == "totals" {
			sideLabel = "Under" // our model only bets under currently
			marketLabel = "O/U"
		} else if b.Market == "spreads" {
			marketLabel = "PL"
		}
		views = append(views, map[string]any{
			"BetID":     b.BetID,
			"Side":      sideLabel,
			"Market":    marketLabel,
			"Stake":     b.Stake,
			"Odds":      b.Odds,
			"IsWinning": b.IsWinning,
		})
	}
	return views
}

func statusLabel(g livescores.LiveGame) string {
	switch g.GameState {
	case livescores.StateLive, livescores.StateCritical:
		if g.InIntermission {
			return fmt.Sprintf("INT %s", g.PeriodLabel)
		}
		return fmt.Sprintf("%s %s", g.PeriodLabel, g.Clock)
	case livescores.StateOff, livescores.StateFinal:
		if g.PeriodLabel == "OT" {
			return "Final/OT"
		}
		if g.PeriodLabel == "SO" {
			return "Final/SO"
		}
		return "Final"
	case livescores.StatePregame:
		return "Pre-game"
	default:
		return g.StartTimeUTC.Format("3:04 PM")
	}
}

func gameSortKey(g livescores.LiveGame) int {
	hasBets := g.HasBets()
	switch {
	case hasBets && g.IsLive():
		return 0
	case hasBets:
		return 1
	case g.IsLive():
		return 2
	case g.GameState == livescores.StateFuture || g.GameState == livescores.StatePregame:
		return 3
	default:
		return 4
	}
}

func formatOdds(odds int) string {
	if odds >= 0 {
		return fmt.Sprintf("+%d", odds)
	}
	return fmt.Sprintf("%d", odds)
}
