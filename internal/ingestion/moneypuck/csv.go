package moneypuck

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TeamGameRow is a parsed row from the MoneyPuck teams CSV.
type TeamGameRow struct {
	Season                          int32
	GameID                          string
	Team                            string
	Opponent                        string
	HomeOrAway                      string
	GameDate                        pgtype.Date
	Situation                       string
	IsPlayoff                       bool
	XgoalsPercentage                *float64
	XgoalsFor                       *float64
	XgoalsAgainst                   *float64
	ScoreVenueAdjustedXgoalsFor     *float64
	ScoreVenueAdjustedXgoalsAgainst *float64
	CorsiPercentage                 *float64
	FenwickPercentage               *float64
	ShotsOnGoalFor                  *float64
	ShotsOnGoalAgainst              *float64
	ShotAttemptsFor                 *float64
	ShotAttemptsAgainst             *float64
	HighDangerShotsFor              *float64
	HighDangerShotsAgainst          *float64
	HighDangerXgoalsFor             *float64
	HighDangerXgoalsAgainst         *float64
	GoalsFor                        *float64
	GoalsAgainst                    *float64
}

// GoalieGameRow is a parsed row from the MoneyPuck goalies CSV.
type GoalieGameRow struct {
	Season           int32
	GameID           string
	PlayerID         string
	Name             string
	Team             string
	Opponent         string
	HomeOrAway       string
	GameDate         pgtype.Date
	Situation        string
	Icetime          *float64
	Xgoals           *float64
	Goals            *float64
	Gsax             *float64
	ShotsAgainst     *float64
	HighDangerXgoals *float64
	HighDangerGoals  *float64
}

// OddsRow is a parsed row from the NHL odds CSV.
type OddsRow struct {
	Season         string
	GameDate       pgtype.Date
	CommenceTime   time.Time
	HomeTeam       string // Canonical abbreviation
	AwayTeam       string // Canonical abbreviation
	HomeScore      float64
	AwayScore      float64
	HomeMoneyLine  int
	AwayMoneyLine  int
	HomeSpread     float64
	AwaySpread     float64
	HomeSpreadLine int
	AwaySpreadLine int
	OverUnder      float64
	OverLine       int
	UnderLine      int
}

// columnIndex maps header names to their 0-based column index.
type columnIndex map[string]int

func buildColumnIndex(headers []string) columnIndex {
	idx := make(columnIndex, len(headers))
	for i, h := range headers {
		idx[strings.TrimSpace(h)] = i
	}
	return idx
}

func (ci columnIndex) require(names ...string) error {
	for _, name := range names {
		if _, ok := ci[name]; !ok {
			return fmt.Errorf("missing required column: %q", name)
		}
	}
	return nil
}

func (ci columnIndex) str(row []string, col string) string {
	i, ok := ci[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func (ci columnIndex) float(row []string, col string) *float64 {
	s := ci.str(row, col)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func (ci columnIndex) intVal(row []string, col string) (int, error) {
	s := ci.str(row, col)
	if s == "" {
		return 0, fmt.Errorf("empty value for %s", col)
	}
	// Handle float-formatted ints (e.g. "155.0")
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s as int: %w", col, err)
		}
		return int(f), nil
	}
	return strconv.Atoi(s)
}

func (ci columnIndex) int32Val(row []string, col string) (int32, error) {
	v, err := ci.intVal(row, col)
	return int32(v), err
}

// parseMoneyPuckDate parses YYYYMMDD integer format to pgtype.Date.
func parseMoneyPuckDate(s string) (pgtype.Date, error) {
	if len(s) != 8 {
		return pgtype.Date{}, fmt.Errorf("invalid date format %q (expected YYYYMMDD)", s)
	}
	t, err := time.Parse("20060102", s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// parseUSDate parses M/D/YYYY format to pgtype.Date.
func parseUSDate(s string) (pgtype.Date, error) {
	t, err := time.Parse("1/2/2006", s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// AmericanToImpliedProbability converts American odds to implied probability.
func AmericanToImpliedProbability(odds int) float64 {
	if odds > 0 {
		return 100.0 / (float64(odds) + 100.0)
	}
	absOdds := math.Abs(float64(odds))
	return absOdds / (absOdds + 100.0)
}

// TeamCSVReader reads MoneyPuck team game CSVs row by row.
type TeamCSVReader struct {
	reader  *csv.Reader
	idx     columnIndex
	teamMap TeamMap
	filter  *int32 // optional season filter
}

var teamRequiredCols = []string{
	"team", "season", "gameId", "playerTeam", "opposingTeam",
	"home_or_away", "gameDate", "position", "situation",
	"xGoalsPercentage", "xGoalsFor", "xGoalsAgainst",
	"scoreVenueAdjustedxGoalsFor", "scoreVenueAdjustedxGoalsAgainst",
	"corsiPercentage", "fenwickPercentage",
	"shotsOnGoalFor", "shotsOnGoalAgainst",
	"shotAttemptsFor", "shotAttemptsAgainst",
	"highDangerShotsFor", "highDangerShotsAgainst",
	"highDangerxGoalsFor", "highDangerxGoalsAgainst",
	"goalsFor", "goalsAgainst",
	"playoffGame",
}

// NewTeamCSVReader creates a reader for MoneyPuck team CSVs.
func NewTeamCSVReader(r io.Reader, tm TeamMap, seasonFilter *int32) (*TeamCSVReader, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := buildColumnIndex(headers)
	if err := idx.require(teamRequiredCols...); err != nil {
		return nil, err
	}

	return &TeamCSVReader{reader: cr, idx: idx, teamMap: tm, filter: seasonFilter}, nil
}

// Next reads the next valid team game row. Returns nil, io.EOF when done.
func (r *TeamCSVReader) Next() (*TeamGameRow, error) {
	for {
		record, err := r.reader.Read()
		if err != nil {
			return nil, err
		}

		// Only "Team Level" position rows
		pos := r.idx.str(record, "position")
		if pos != "Team Level" {
			continue
		}

		// Only 5on5 and all situations
		sit := r.idx.str(record, "situation")
		if sit != "5on5" && sit != "all" {
			continue
		}

		season, err := r.idx.int32Val(record, "season")
		if err != nil {
			continue
		}
		if r.filter != nil && season != *r.filter {
			continue
		}

		team := r.idx.str(record, "team")
		canonical, err := r.teamMap.Canonical(team)
		if err != nil {
			return nil, fmt.Errorf("row team %q: %w", team, err)
		}

		opponent := r.idx.str(record, "opposingTeam")
		canonOpp, err := r.teamMap.Canonical(opponent)
		if err != nil {
			return nil, fmt.Errorf("row opponent %q: %w", opponent, err)
		}

		gameDate, err := parseMoneyPuckDate(r.idx.str(record, "gameDate"))
		if err != nil {
			return nil, fmt.Errorf("row date: %w", err)
		}

		playoffStr := r.idx.str(record, "playoffGame")
		isPlayoff := playoffStr == "1"

		homeOrAway := strings.ToUpper(r.idx.str(record, "home_or_away"))
		if homeOrAway != "HOME" && homeOrAway != "AWAY" {
			continue
		}

		return &TeamGameRow{
			Season:                          season,
			GameID:                          r.idx.str(record, "gameId"),
			Team:                            canonical,
			Opponent:                        canonOpp,
			HomeOrAway:                      homeOrAway,
			GameDate:                        gameDate,
			Situation:                       sit,
			IsPlayoff:                       isPlayoff,
			XgoalsPercentage:                r.idx.float(record, "xGoalsPercentage"),
			XgoalsFor:                       r.idx.float(record, "xGoalsFor"),
			XgoalsAgainst:                   r.idx.float(record, "xGoalsAgainst"),
			ScoreVenueAdjustedXgoalsFor:     r.idx.float(record, "scoreVenueAdjustedxGoalsFor"),
			ScoreVenueAdjustedXgoalsAgainst: r.idx.float(record, "scoreVenueAdjustedxGoalsAgainst"),
			CorsiPercentage:                 r.idx.float(record, "corsiPercentage"),
			FenwickPercentage:               r.idx.float(record, "fenwickPercentage"),
			ShotsOnGoalFor:                  r.idx.float(record, "shotsOnGoalFor"),
			ShotsOnGoalAgainst:              r.idx.float(record, "shotsOnGoalAgainst"),
			ShotAttemptsFor:                 r.idx.float(record, "shotAttemptsFor"),
			ShotAttemptsAgainst:             r.idx.float(record, "shotAttemptsAgainst"),
			HighDangerShotsFor:              r.idx.float(record, "highDangerShotsFor"),
			HighDangerShotsAgainst:          r.idx.float(record, "highDangerShotsAgainst"),
			HighDangerXgoalsFor:             r.idx.float(record, "highDangerxGoalsFor"),
			HighDangerXgoalsAgainst:         r.idx.float(record, "highDangerxGoalsAgainst"),
			GoalsFor:                        r.idx.float(record, "goalsFor"),
			GoalsAgainst:                    r.idx.float(record, "goalsAgainst"),
		}, nil
	}
}

// GoalieCSVReader reads MoneyPuck goalie game CSVs row by row.
type GoalieCSVReader struct {
	reader  *csv.Reader
	idx     columnIndex
	teamMap TeamMap
	filter  *int32
}

var goalieRequiredCols = []string{
	"playerId", "name", "gameId", "season", "playerTeam", "opposingTeam",
	"home_or_away", "gameDate", "position", "situation",
	"icetime", "xGoals", "goals", "unblocked_shot_attempts",
	"highDangerxGoals", "highDangerGoals",
}

// NewGoalieCSVReader creates a reader for MoneyPuck goalie CSVs.
func NewGoalieCSVReader(r io.Reader, tm TeamMap, seasonFilter *int32) (*GoalieCSVReader, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := buildColumnIndex(headers)
	if err := idx.require(goalieRequiredCols...); err != nil {
		return nil, err
	}

	return &GoalieCSVReader{reader: cr, idx: idx, teamMap: tm, filter: seasonFilter}, nil
}

// Next reads the next valid goalie game row. Returns nil, io.EOF when done.
func (r *GoalieCSVReader) Next() (*GoalieGameRow, error) {
	for {
		record, err := r.reader.Read()
		if err != nil {
			return nil, err
		}

		// Only goalie rows
		pos := r.idx.str(record, "position")
		if pos != "G" {
			continue
		}

		sit := r.idx.str(record, "situation")
		if sit != "5on5" && sit != "all" {
			continue
		}

		season, err := r.idx.int32Val(record, "season")
		if err != nil {
			continue
		}
		if r.filter != nil && season != *r.filter {
			continue
		}

		team := r.idx.str(record, "playerTeam")
		canonical, err := r.teamMap.Canonical(team)
		if err != nil {
			return nil, fmt.Errorf("row team %q: %w", team, err)
		}

		opponent := r.idx.str(record, "opposingTeam")
		canonOpp, err := r.teamMap.Canonical(opponent)
		if err != nil {
			return nil, fmt.Errorf("row opponent %q: %w", opponent, err)
		}

		gameDate, err := parseMoneyPuckDate(r.idx.str(record, "gameDate"))
		if err != nil {
			return nil, fmt.Errorf("row date: %w", err)
		}

		homeOrAway := strings.ToUpper(r.idx.str(record, "home_or_away"))
		if homeOrAway != "HOME" && homeOrAway != "AWAY" {
			continue
		}

		xgoals := r.idx.float(record, "xGoals")
		goals := r.idx.float(record, "goals")
		var gsax *float64
		if xgoals != nil && goals != nil {
			v := *xgoals - *goals
			gsax = &v
		}

		return &GoalieGameRow{
			Season:           season,
			GameID:           r.idx.str(record, "gameId"),
			PlayerID:         r.idx.str(record, "playerId"),
			Name:             r.idx.str(record, "name"),
			Team:             canonical,
			Opponent:         canonOpp,
			HomeOrAway:       homeOrAway,
			GameDate:         gameDate,
			Situation:        sit,
			Icetime:          r.idx.float(record, "icetime"),
			Xgoals:           xgoals,
			Goals:            goals,
			Gsax:             gsax,
			ShotsAgainst:     r.idx.float(record, "unblocked_shot_attempts"),
			HighDangerXgoals: r.idx.float(record, "highDangerxGoals"),
			HighDangerGoals:  r.idx.float(record, "highDangerGoals"),
		}, nil
	}
}

// OddsCSVReader reads NHL odds CSVs row by row.
type OddsCSVReader struct {
	reader  *csv.Reader
	idx     columnIndex
	teamMap TeamMap
}

var oddsRequiredCols = []string{
	"season", "date", "home_team", "away_team",
	"home_score", "away_score",
	"home_money_line", "away_money_line",
	"home_point_spread", "away_point_spread",
	"home_point_spread_line", "away_point_spread_line",
	"over_under", "over_line", "under_line",
}

// NewOddsCSVReader creates a reader for NHL odds CSVs.
func NewOddsCSVReader(r io.Reader, tm TeamMap) (*OddsCSVReader, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := buildColumnIndex(headers)
	if err := idx.require(oddsRequiredCols...); err != nil {
		return nil, err
	}

	return &OddsCSVReader{reader: cr, idx: idx, teamMap: tm}, nil
}

// Next reads the next odds row. Returns nil, io.EOF when done.
func (r *OddsCSVReader) Next() (*OddsRow, error) {
	record, err := r.reader.Read()
	if err != nil {
		return nil, err
	}

	gameDate, err := parseUSDate(r.idx.str(record, "date"))
	if err != nil {
		return nil, fmt.Errorf("row date: %w", err)
	}

	homeSnake := r.idx.str(record, "home_team")
	home, err := r.teamMap.FromSnakeName(homeSnake)
	if err != nil {
		return nil, fmt.Errorf("row home_team %q: %w", homeSnake, err)
	}

	awaySnake := r.idx.str(record, "away_team")
	away, err := r.teamMap.FromSnakeName(awaySnake)
	if err != nil {
		return nil, fmt.Errorf("row away_team %q: %w", awaySnake, err)
	}

	homeScore, err := r.idx.intVal(record, "home_score")
	if err != nil {
		return nil, fmt.Errorf("row home_score: %w", err)
	}
	awayScore, err := r.idx.intVal(record, "away_score")
	if err != nil {
		return nil, fmt.Errorf("row away_score: %w", err)
	}

	homeML, err := r.idx.intVal(record, "home_money_line")
	if err != nil {
		return nil, fmt.Errorf("row home_money_line: %w", err)
	}
	awayML, err := r.idx.intVal(record, "away_money_line")
	if err != nil {
		return nil, fmt.Errorf("row away_money_line: %w", err)
	}

	homeSpreadF := r.idx.float(record, "home_point_spread")
	awaySpreadF := r.idx.float(record, "away_point_spread")
	homeSpread := 0.0
	awaySpread := 0.0
	if homeSpreadF != nil {
		homeSpread = *homeSpreadF
	}
	if awaySpreadF != nil {
		awaySpread = *awaySpreadF
	}

	homeSpreadLine, err := r.idx.intVal(record, "home_point_spread_line")
	if err != nil {
		return nil, fmt.Errorf("row home_point_spread_line: %w", err)
	}
	awaySpreadLine, err := r.idx.intVal(record, "away_point_spread_line")
	if err != nil {
		return nil, fmt.Errorf("row away_point_spread_line: %w", err)
	}

	ouF := r.idx.float(record, "over_under")
	ou := 0.0
	if ouF != nil {
		ou = *ouF
	}

	overLine, err := r.idx.intVal(record, "over_line")
	if err != nil {
		return nil, fmt.Errorf("row over_line: %w", err)
	}
	underLine, err := r.idx.intVal(record, "under_line")
	if err != nil {
		return nil, fmt.Errorf("row under_line: %w", err)
	}

	// Use 7pm ET as a reasonable default game time
	commenceTime := gameDate.Time.Add(19 * time.Hour)

	return &OddsRow{
		Season:         r.idx.str(record, "season"),
		GameDate:       gameDate,
		CommenceTime:   commenceTime,
		HomeTeam:       home,
		AwayTeam:       away,
		HomeScore:      float64(homeScore),
		AwayScore:      float64(awayScore),
		HomeMoneyLine:  homeML,
		AwayMoneyLine:  awayML,
		HomeSpread:     homeSpread,
		AwaySpread:     awaySpread,
		HomeSpreadLine: homeSpreadLine,
		AwaySpreadLine: awaySpreadLine,
		OverUnder:      ou,
		OverLine:       overLine,
		UnderLine:      underLine,
	}, nil
}

// HomeImpliedProbability returns the implied probability for the home moneyline.
func (r OddsRow) HomeImpliedProbability() float64 {
	return AmericanToImpliedProbability(r.HomeMoneyLine)
}

// AwayImpliedProbability returns the implied probability for the away moneyline.
func (r OddsRow) AwayImpliedProbability() float64 {
	return AmericanToImpliedProbability(r.AwayMoneyLine)
}
