package weather

import (
	"testing"
	"time"
)

func TestLookupVenueByRoofType(t *testing.T) {
	tests := []struct {
		name       string
		sport      string
		homeTeam   string
		commenceAt time.Time
		wantVenue  string
		wantRoof   RoofType
		wantPolicy RoofWeatherPolicy
	}{
		{
			name:       "outdoor",
			sport:      "MLB",
			homeTeam:   "Boston Red Sox",
			commenceAt: time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC),
			wantVenue:  "Fenway Park",
			wantRoof:   RoofTypeOutdoor,
			wantPolicy: RoofWeatherPolicyOutdoor,
		},
		{
			name:       "dome",
			sport:      "MLB",
			homeTeam:   "Tampa Bay Rays",
			commenceAt: time.Date(2026, time.March, 12, 23, 40, 0, 0, time.UTC),
			wantVenue:  "Tropicana Field",
			wantRoof:   RoofTypeDome,
			wantPolicy: RoofWeatherPolicyFixedIndoor,
		},
		{
			name:       "retractable",
			sport:      "MLB",
			homeTeam:   "Toronto Blue Jays",
			commenceAt: time.Date(2026, time.March, 12, 23, 7, 0, 0, time.UTC),
			wantVenue:  "Rogers Centre",
			wantRoof:   RoofTypeRetractable,
			wantPolicy: RoofWeatherPolicyRetractableUnknown,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			venue, ok := LookupVenue(tc.sport, tc.homeTeam, tc.commenceAt)
			if !ok {
				t.Fatalf("LookupVenue(%q, %q) not found", tc.sport, tc.homeTeam)
			}
			if venue.Name != tc.wantVenue {
				t.Fatalf("venue = %q, want %q", venue.Name, tc.wantVenue)
			}
			if venue.RoofType != tc.wantRoof {
				t.Fatalf("roof type = %q, want %q", venue.RoofType, tc.wantRoof)
			}
			if venue.WeatherPolicy() != tc.wantPolicy {
				t.Fatalf("weather policy = %q, want %q", venue.WeatherPolicy(), tc.wantPolicy)
			}
		})
	}
}

func TestLookupVenueAthleticsDateOverrideAppliesToAliases(t *testing.T) {
	insideOverride := time.Date(2026, time.June, 10, 2, 0, 0, 0, time.UTC)
	outsideOverride := time.Date(2026, time.June, 20, 2, 0, 0, 0, time.UTC)

	for _, homeTeam := range []string{"Athletics", "Oakland Athletics"} {
		homeTeam := homeTeam
		t.Run(homeTeam, func(t *testing.T) {
			overrideVenue, ok := LookupVenue("MLB", homeTeam, insideOverride)
			if !ok {
				t.Fatalf("LookupVenue(%q, inside override) not found", homeTeam)
			}
			if overrideVenue.Name != "Las Vegas Ballpark" {
				t.Fatalf("override venue = %q, want Las Vegas Ballpark", overrideVenue.Name)
			}
			if overrideVenue.RoofType != RoofTypeOutdoor {
				t.Fatalf("override roof type = %q, want %q", overrideVenue.RoofType, RoofTypeOutdoor)
			}

			defaultVenue, ok := LookupVenue("MLB", homeTeam, outsideOverride)
			if !ok {
				t.Fatalf("LookupVenue(%q, outside override) not found", homeTeam)
			}
			if defaultVenue.Name != "Sutter Health Park" {
				t.Fatalf("default venue = %q, want Sutter Health Park", defaultVenue.Name)
			}
		})
	}
}
