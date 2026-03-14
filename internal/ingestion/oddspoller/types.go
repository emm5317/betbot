package oddspoller

import (
	"encoding/json"
	"time"
)

type APIGame struct {
	ID           string          `json:"id"`
	SportKey     string          `json:"sport_key"`
	SportTitle   string          `json:"sport_title"`
	CommenceTime time.Time       `json:"commence_time"`
	HomeTeam     string          `json:"home_team"`
	AwayTeam     string          `json:"away_team"`
	Bookmakers   []APIBookmaker  `json:"bookmakers"`
	Completed    *bool           `json:"completed,omitempty"`
	Scores       json.RawMessage `json:"scores,omitempty"`
	Raw          json.RawMessage `json:"-"`
}

type APIBookmaker struct {
	Key        string      `json:"key"`
	Title      string      `json:"title"`
	LastUpdate time.Time   `json:"last_update"`
	Markets    []APIMarket `json:"markets"`
}

type APIMarket struct {
	Key        string       `json:"key"`
	LastUpdate time.Time    `json:"last_update"`
	Outcomes   []APIOutcome `json:"outcomes"`
}

type APIOutcome struct {
	Name  string   `json:"name"`
	Price int      `json:"price"`
	Point *float64 `json:"point,omitempty"`
}

type CanonicalGame struct {
	Source       string
	ExternalID   string
	Sport        string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
}

type CanonicalOddsSnapshot struct {
	Source             string
	GameExternalID     string
	BookKey            string
	BookName           string
	MarketKey          string
	MarketName         string
	OutcomeName        string
	OutcomeSide        string
	PriceAmerican      int
	Point              *float64
	ImpliedProbability float64
	SnapshotHash       string
	CapturedAt         time.Time
	RawJSON            json.RawMessage
}

type NormalizedPayload struct {
	Games     []CanonicalGame
	Snapshots []CanonicalOddsSnapshot
}

type PollMetrics struct {
	GamesSeen     int
	SnapshotsSeen int
	Inserts       int
	DedupSkips    int
}
