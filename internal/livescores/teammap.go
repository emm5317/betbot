package livescores

import (
	"strings"
	"time"

	"betbot/internal/ingestion/moneypuck"
)

// NormalizeNHLGame converts an NHL API game to a LiveGame using the TeamMap
// to resolve full Odds API team names for bet matching.
func NormalizeNHLGame(game APIGame, tm moneypuck.TeamMap) LiveGame {
	homeName := resolveTeamName(game.HomeTeam.Abbrev, tm)
	awayName := resolveTeamName(game.AwayTeam.Abbrev, tm)

	startTime, _ := time.Parse(time.RFC3339, game.StartTimeUTC)

	return LiveGame{
		NHLID:          game.ID,
		GameState:      game.GameState,
		Period:         game.Period,
		PeriodLabel:    periodLabel(game.Period, game.PeriodDescriptor.PeriodType),
		Clock:          game.Clock.TimeRemaining,
		InIntermission: game.Clock.InIntermission,

		HomeAbbrev: game.HomeTeam.Abbrev,
		HomeName:   homeName,
		HomeScore:  game.HomeTeam.Score,
		HomeSOG:    game.HomeTeam.SOG,
		HomeRecord: game.HomeTeam.Record,

		AwayAbbrev: game.AwayTeam.Abbrev,
		AwayName:   awayName,
		AwayScore:  game.AwayTeam.Score,
		AwaySOG:    game.AwayTeam.SOG,
		AwayRecord: game.AwayTeam.Record,

		StartTimeUTC: startTime,
	}
}

func resolveTeamName(abbrev string, tm moneypuck.TeamMap) string {
	name, err := tm.ToOddsAPIName(abbrev)
	if err != nil {
		return abbrev
	}
	return name
}

func periodLabel(period int, periodType string) string {
	if strings.EqualFold(periodType, "SO") || period == 5 {
		return "SO"
	}
	if strings.EqualFold(periodType, "OT") || period == 4 {
		return "OT"
	}
	switch period {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return "OT"
	}
}
