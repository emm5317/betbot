package moneypuck

import (
	"fmt"
	"strings"
)

// TeamEntry holds the canonical abbreviation and all known name variants for an NHL team.
type TeamEntry struct {
	Abbrev       string // Canonical 3-letter abbreviation (modern MoneyPuck format)
	OddsAPIName  string // The Odds API full name (e.g. "Tampa Bay Lightning")
	SnakeName    string // Snake_case name used in odds CSVs (e.g. "tampa_bay_lightning")
	LegacyAbbrev string // Pre-2021 MoneyPuck abbreviation if different (e.g. "T.B")
}

var teams = []TeamEntry{
	{Abbrev: "ANA", OddsAPIName: "Anaheim Ducks", SnakeName: "anaheim_ducks"},
	{Abbrev: "ARI", OddsAPIName: "Arizona Coyotes", SnakeName: "arizona_coyotes"},
	{Abbrev: "ATL", OddsAPIName: "Atlanta Thrashers", SnakeName: "atlanta_thrashers"},
	{Abbrev: "BOS", OddsAPIName: "Boston Bruins", SnakeName: "boston_bruins"},
	{Abbrev: "BUF", OddsAPIName: "Buffalo Sabres", SnakeName: "buffalo_sabres"},
	{Abbrev: "CAR", OddsAPIName: "Carolina Hurricanes", SnakeName: "carolina_hurricanes"},
	{Abbrev: "CBJ", OddsAPIName: "Columbus Blue Jackets", SnakeName: "columbus_blue_jackets"},
	{Abbrev: "CGY", OddsAPIName: "Calgary Flames", SnakeName: "calgary_flames"},
	{Abbrev: "CHI", OddsAPIName: "Chicago Blackhawks", SnakeName: "chicago_blackhawks"},
	{Abbrev: "COL", OddsAPIName: "Colorado Avalanche", SnakeName: "colorado_avalanche"},
	{Abbrev: "DAL", OddsAPIName: "Dallas Stars", SnakeName: "dallas_stars"},
	{Abbrev: "DET", OddsAPIName: "Detroit Red Wings", SnakeName: "detroit_red_wings"},
	{Abbrev: "EDM", OddsAPIName: "Edmonton Oilers", SnakeName: "edmonton_oilers"},
	{Abbrev: "FLA", OddsAPIName: "Florida Panthers", SnakeName: "florida_panthers"},
	{Abbrev: "LAK", OddsAPIName: "Los Angeles Kings", SnakeName: "los_angeles_kings", LegacyAbbrev: "L.A"},
	{Abbrev: "MIN", OddsAPIName: "Minnesota Wild", SnakeName: "minnesota_wild"},
	{Abbrev: "MTL", OddsAPIName: "Montreal Canadiens", SnakeName: "montreal_canadiens"},
	{Abbrev: "NJD", OddsAPIName: "New Jersey Devils", SnakeName: "new_jersey_devils", LegacyAbbrev: "N.J"},
	{Abbrev: "NSH", OddsAPIName: "Nashville Predators", SnakeName: "nashville_predators"},
	{Abbrev: "NYI", OddsAPIName: "New York Islanders", SnakeName: "new_york_islanders"},
	{Abbrev: "NYR", OddsAPIName: "New York Rangers", SnakeName: "new_york_rangers"},
	{Abbrev: "OTT", OddsAPIName: "Ottawa Senators", SnakeName: "ottawa_senators"},
	{Abbrev: "PHI", OddsAPIName: "Philadelphia Flyers", SnakeName: "philadelphia_flyers"},
	{Abbrev: "PIT", OddsAPIName: "Pittsburgh Penguins", SnakeName: "pittsburgh_penguins"},
	{Abbrev: "SEA", OddsAPIName: "Seattle Kraken", SnakeName: "seattle_kraken"},
	{Abbrev: "SJS", OddsAPIName: "San Jose Sharks", SnakeName: "san_jose_sharks", LegacyAbbrev: "S.J"},
	{Abbrev: "STL", OddsAPIName: "St Louis Blues", SnakeName: "st._louis_blues"},
	{Abbrev: "TBL", OddsAPIName: "Tampa Bay Lightning", SnakeName: "tampa_bay_lightning", LegacyAbbrev: "T.B"},
	{Abbrev: "TOR", OddsAPIName: "Toronto Maple Leafs", SnakeName: "toronto_maple_leafs"},
	{Abbrev: "UTA", OddsAPIName: "Utah Hockey Club", SnakeName: "utah_hockey_club"},
	{Abbrev: "VAN", OddsAPIName: "Vancouver Canucks", SnakeName: "vancouver_canucks"},
	{Abbrev: "VGK", OddsAPIName: "Vegas Golden Knights", SnakeName: "vegas_golden_knights"},
	{Abbrev: "WPG", OddsAPIName: "Winnipeg Jets", SnakeName: "winnipeg_jets"},
	{Abbrev: "WSH", OddsAPIName: "Washington Capitals", SnakeName: "washington_capitals"},
}

// nameAliases maps alternate full names to a canonical abbreviation.
// Used for franchise renames that don't change the abbreviation.
var nameAliases = map[string]string{
	"utah mammoth":  "UTA", // 2025-26 rename from Utah Hockey Club
	"st. louis blues": "STL", // Period-style variant (used in as-played CSVs)
}

// TeamMap provides bidirectional lookups between all NHL team naming formats.
type TeamMap struct {
	byAbbrev  map[string]TeamEntry
	bySnake   map[string]TeamEntry
	byOddsAPI map[string]TeamEntry
}

// NewTeamMap builds the lookup indexes.
func NewTeamMap() TeamMap {
	m := TeamMap{
		byAbbrev:  make(map[string]TeamEntry, len(teams)*2),
		bySnake:   make(map[string]TeamEntry, len(teams)),
		byOddsAPI: make(map[string]TeamEntry, len(teams)+len(nameAliases)),
	}
	for _, t := range teams {
		m.byAbbrev[t.Abbrev] = t
		if t.LegacyAbbrev != "" {
			m.byAbbrev[t.LegacyAbbrev] = t
		}
		m.bySnake[t.SnakeName] = t
		m.byOddsAPI[strings.ToLower(t.OddsAPIName)] = t
	}
	// Register aliases — they resolve to the same entry as the canonical abbreviation.
	for alias, abbrev := range nameAliases {
		if entry, ok := m.byAbbrev[abbrev]; ok {
			m.byOddsAPI[alias] = entry
		}
	}
	return m
}

// Canonical normalizes any MoneyPuck abbreviation (including legacy) to the
// canonical 3-letter form (e.g. "T.B" → "TBL", "LAK" → "LAK").
func (m TeamMap) Canonical(abbrev string) (string, error) {
	entry, ok := m.byAbbrev[strings.TrimSpace(abbrev)]
	if !ok {
		return "", fmt.Errorf("unknown MoneyPuck abbreviation: %q", abbrev)
	}
	return entry.Abbrev, nil
}

// FromSnakeName resolves a snake_case team name to its canonical abbreviation.
func (m TeamMap) FromSnakeName(snake string) (string, error) {
	entry, ok := m.bySnake[strings.TrimSpace(strings.ToLower(snake))]
	if !ok {
		return "", fmt.Errorf("unknown snake_case team name: %q", snake)
	}
	return entry.Abbrev, nil
}

// FromOddsAPIName resolves a full Odds API team name to its canonical abbreviation.
// Also resolves name aliases (e.g. "Utah Mammoth" → "UTA").
func (m TeamMap) FromOddsAPIName(name string) (string, error) {
	entry, ok := m.byOddsAPI[strings.TrimSpace(strings.ToLower(name))]
	if !ok {
		return "", fmt.Errorf("unknown Odds API team name: %q", name)
	}
	return entry.Abbrev, nil
}

// ToOddsAPIName converts a canonical abbreviation to the Odds API full name.
func (m TeamMap) ToOddsAPIName(abbrev string) (string, error) {
	entry, ok := m.byAbbrev[strings.TrimSpace(abbrev)]
	if !ok {
		return "", fmt.Errorf("unknown abbreviation: %q", abbrev)
	}
	return entry.OddsAPIName, nil
}

// ToSnakeName converts a canonical abbreviation to the snake_case format.
func (m TeamMap) ToSnakeName(abbrev string) (string, error) {
	entry, ok := m.byAbbrev[strings.TrimSpace(abbrev)]
	if !ok {
		return "", fmt.Errorf("unknown abbreviation: %q", abbrev)
	}
	return entry.SnakeName, nil
}

// Entry returns the full TeamEntry for any known abbreviation.
func (m TeamMap) Entry(abbrev string) (TeamEntry, bool) {
	entry, ok := m.byAbbrev[strings.TrimSpace(abbrev)]
	return entry, ok
}
