package moneypuck

import (
	"testing"
)

func TestCanonical(t *testing.T) {
	m := NewTeamMap()

	tests := []struct {
		input string
		want  string
	}{
		{"TBL", "TBL"},
		{"T.B", "TBL"},
		{"LAK", "LAK"},
		{"L.A", "LAK"},
		{"SJS", "SJS"},
		{"S.J", "SJS"},
		{"NJD", "NJD"},
		{"N.J", "NJD"},
		{"NYR", "NYR"},
		{"ANA", "ANA"},
		{"ATL", "ATL"},
		{"UTA", "UTA"},
		{"WPG", "WPG"},
	}

	for _, tt := range tests {
		got, err := m.Canonical(tt.input)
		if err != nil {
			t.Errorf("Canonical(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Canonical(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCanonicalUnknown(t *testing.T) {
	m := NewTeamMap()
	_, err := m.Canonical("XXX")
	if err == nil {
		t.Error("expected error for unknown abbreviation")
	}
}

func TestFromSnakeName(t *testing.T) {
	m := NewTeamMap()

	tests := []struct {
		input string
		want  string
	}{
		{"tampa_bay_lightning", "TBL"},
		{"los_angeles_kings", "LAK"},
		{"san_jose_sharks", "SJS"},
		{"new_jersey_devils", "NJD"},
		{"st._louis_blues", "STL"},
		{"vegas_golden_knights", "VGK"},
		{"seattle_kraken", "SEA"},
		{"utah_hockey_club", "UTA"},
		{"arizona_coyotes", "ARI"},
	}

	for _, tt := range tests {
		got, err := m.FromSnakeName(tt.input)
		if err != nil {
			t.Errorf("FromSnakeName(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("FromSnakeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFromOddsAPIName(t *testing.T) {
	m := NewTeamMap()

	tests := []struct {
		input string
		want  string
	}{
		{"Tampa Bay Lightning", "TBL"},
		{"Los Angeles Kings", "LAK"},
		{"New York Rangers", "NYR"},
		{"St Louis Blues", "STL"},
	}

	for _, tt := range tests {
		got, err := m.FromOddsAPIName(tt.input)
		if err != nil {
			t.Errorf("FromOddsAPIName(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("FromOddsAPIName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	m := NewTeamMap()

	for _, entry := range teams {
		abbrev, err := m.FromSnakeName(entry.SnakeName)
		if err != nil {
			t.Errorf("FromSnakeName(%q) error: %v", entry.SnakeName, err)
			continue
		}
		name, err := m.ToOddsAPIName(abbrev)
		if err != nil {
			t.Errorf("ToOddsAPIName(%q) error: %v", abbrev, err)
			continue
		}
		if name != entry.OddsAPIName {
			t.Errorf("round trip %q → %q → %q, want %q", entry.SnakeName, abbrev, name, entry.OddsAPIName)
		}
	}
}

func TestAllTeamsHaveSnakeNames(t *testing.T) {
	m := NewTeamMap()
	for _, entry := range teams {
		if entry.SnakeName == "" {
			t.Errorf("team %s has no snake name", entry.Abbrev)
		}
		_, err := m.FromSnakeName(entry.SnakeName)
		if err != nil {
			t.Errorf("snake name %q not resolvable: %v", entry.SnakeName, err)
		}
	}
}
