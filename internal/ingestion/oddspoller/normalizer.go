package oddspoller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Normalizer struct {
	source string
}

func NewNormalizer(source string) *Normalizer {
	return &Normalizer{source: source}
}

func (n *Normalizer) Normalize(games []APIGame, capturedAt time.Time) (*NormalizedPayload, error) {
	payload := &NormalizedPayload{
		Games:     make([]CanonicalGame, 0, len(games)),
		Snapshots: make([]CanonicalOddsSnapshot, 0, len(games)*6),
	}

	for _, game := range games {
		if strings.TrimSpace(game.ID) == "" {
			return nil, fmt.Errorf("game id is required")
		}
		if strings.TrimSpace(game.HomeTeam) == "" || strings.TrimSpace(game.AwayTeam) == "" {
			return nil, fmt.Errorf("game %s missing teams", game.ID)
		}

		payload.Games = append(payload.Games, CanonicalGame{
			Source:       n.source,
			ExternalID:   game.ID,
			Sport:        normalizeSport(game.SportKey),
			HomeTeam:     game.HomeTeam,
			AwayTeam:     game.AwayTeam,
			CommenceTime: game.CommenceTime.UTC(),
		})

		for _, bookmaker := range game.Bookmakers {
			for _, market := range bookmaker.Markets {
				for _, outcome := range market.Outcomes {
					snapshot := CanonicalOddsSnapshot{
						Source:             n.source,
						GameExternalID:     game.ID,
						BookKey:            bookmaker.Key,
						BookName:           bookmaker.Title,
						MarketKey:          market.Key,
						MarketName:         marketDisplayName(market.Key),
						OutcomeName:        outcome.Name,
						OutcomeSide:        inferOutcomeSide(game, market.Key, outcome.Name),
						PriceAmerican:      outcome.Price,
						Point:              outcome.Point,
						ImpliedProbability: impliedProbability(outcome.Price),
						CapturedAt:         capturedAt.UTC(),
						RawJSON:            cloneJSON(game.Raw),
					}
					snapshot.SnapshotHash = snapshotHash(snapshot)
					payload.Snapshots = append(payload.Snapshots, snapshot)
				}
			}
		}
	}

	return payload, nil
}

func normalizeSport(sportKey string) string {
	switch {
	case strings.Contains(sportKey, "mlb"):
		return "MLB"
	case strings.Contains(sportKey, "nba"):
		return "NBA"
	case strings.Contains(sportKey, "nhl"):
		return "NHL"
	case strings.Contains(sportKey, "nfl"):
		return "NFL"
	default:
		return strings.ToUpper(sportKey)
	}
}

func marketDisplayName(key string) string {
	switch key {
	case "h2h":
		return "Moneyline"
	case "spreads":
		return "Spread"
	case "totals":
		return "Total"
	default:
		return key
	}
}

func inferOutcomeSide(game APIGame, marketKey string, outcomeName string) string {
	switch marketKey {
	case "totals":
		lower := strings.ToLower(outcomeName)
		if strings.Contains(lower, "over") {
			return "over"
		}
		if strings.Contains(lower, "under") {
			return "under"
		}
	case "h2h", "spreads":
		if outcomeName == game.HomeTeam {
			return "home"
		}
		if outcomeName == game.AwayTeam {
			return "away"
		}
	}
	return strings.ToLower(strings.ReplaceAll(outcomeName, " ", "_"))
}

func impliedProbability(price int) float64 {
	if price == 0 {
		return 0
	}
	if price > 0 {
		return 100.0 / float64(price+100)
	}
	abs := float64(-price)
	return abs / (abs + 100.0)
}

func snapshotHash(snapshot CanonicalOddsSnapshot) string {
	payload := map[string]any{
		"source":       snapshot.Source,
		"external_id":  snapshot.GameExternalID,
		"book_key":     snapshot.BookKey,
		"market_key":   snapshot.MarketKey,
		"outcome_name": snapshot.OutcomeName,
		"outcome_side": snapshot.OutcomeSide,
		"price":        snapshot.PriceAmerican,
		"point":        snapshot.Point,
	}
	body, _ := json.Marshal(payload)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
