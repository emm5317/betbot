package livescores

import "time"

// APIScoreResponse is the top-level response from GET /v1/score/now.
type APIScoreResponse struct {
	CurrentDate string    `json:"currentDate"`
	Games       []APIGame `json:"games"`
}

// APIGame represents a single game from the NHL score endpoint.
type APIGame struct {
	ID                int              `json:"id"`
	GameState         string           `json:"gameState"`         // FUT, PRE, LIVE, CRIT, OFF, FINAL
	GameScheduleState string           `json:"gameScheduleState"` // OK, CNCL, PPD
	GameType          int              `json:"gameType"`          // 2=regular, 3=playoff
	StartTimeUTC      string           `json:"startTimeUTC"`
	HomeTeam          APITeam          `json:"homeTeam"`
	AwayTeam          APITeam          `json:"awayTeam"`
	Clock             APIClock         `json:"clock"`
	Period            int              `json:"period"`
	PeriodDescriptor  APIPeriodDesc    `json:"periodDescriptor"`
	Venue             APILocalizedName `json:"venue"`
}

// APITeam represents a team in the NHL score response.
type APITeam struct {
	ID     int              `json:"id"`
	Abbrev string           `json:"abbrev"`
	Name   APILocalizedName `json:"name"`
	Logo   string           `json:"logo"`
	Score  int              `json:"score"`
	SOG    int              `json:"sog"`
	Record string           `json:"record"`
}

// APIClock represents the game clock.
type APIClock struct {
	TimeRemaining    string `json:"timeRemaining"`
	SecondsRemaining int    `json:"secondsRemaining"`
	Running          bool   `json:"running"`
	InIntermission   bool   `json:"inIntermission"`
}

// APIPeriodDesc describes the current period.
type APIPeriodDesc struct {
	Number               int    `json:"number"`
	PeriodType           string `json:"periodType"` // REG, OT, SO
	MaxRegulationPeriods int    `json:"maxRegulationPeriods"`
}

// APILocalizedName wraps the NHL API's localized name pattern.
type APILocalizedName struct {
	Default string `json:"default"`
}

// Game states returned by the NHL API.
const (
	StateFuture   = "FUT"
	StatePregame  = "PRE"
	StateLive     = "LIVE"
	StateCritical = "CRIT"
	StateOff      = "OFF"
	StateFinal    = "FINAL"
)

// LiveGame is the normalized domain model for template rendering and bet matching.
type LiveGame struct {
	NHLID          int
	GameState      string // FUT, PRE, LIVE, CRIT, OFF, FINAL
	Period         int
	PeriodLabel    string // "1st", "2nd", "3rd", "OT", "SO"
	Clock          string // "12:34" or ""
	InIntermission bool

	HomeAbbrev string
	HomeName   string // Odds API full name for bet matching
	HomeScore  int
	HomeSOG    int
	HomeRecord string

	AwayAbbrev string
	AwayName   string
	AwayScore  int
	AwaySOG    int
	AwayRecord string

	StartTimeUTC time.Time

	// Bet overlay — populated by the server handler, not the cache.
	Bets []LiveGameBet
}

// LiveGameBet holds bet information overlaid on a live game.
type LiveGameBet struct {
	BetID     int64
	Side      string // "home" or "away"
	Market    string // "h2h", "totals"
	Stake     string // formatted "$40.00"
	Odds      string // "+165"
	IsWinning bool   // true if the bet side is currently winning
}

// IsLive returns true if the game is currently in progress.
func (g LiveGame) IsLive() bool {
	return g.GameState == StateLive || g.GameState == StateCritical
}

// IsComplete returns true if the game has ended.
func (g LiveGame) IsComplete() bool {
	return g.GameState == StateOff || g.GameState == StateFinal
}

// HasBets returns true if any bets are placed on this game.
func (g LiveGame) HasBets() bool {
	return len(g.Bets) > 0
}

// TotalGoals returns the sum of home and away scores.
func (g LiveGame) TotalGoals() int {
	return g.HomeScore + g.AwayScore
}
