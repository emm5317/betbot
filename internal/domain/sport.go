package domain

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

type Sport string

const (
	SportMLB Sport = "MLB"
	SportNBA Sport = "NBA"
	SportNHL Sport = "NHL"
	SportNFL Sport = "NFL"
)

type KellyRange struct {
	Min float64
	Max float64
}

type PollCadence struct {
	Pregame time.Duration
	Live    time.Duration
}

type SeasonWindow struct {
	StartMonth time.Month
	StartDay   int
	EndMonth   time.Month
	EndDay     int
}

func (w SeasonWindow) Contains(at time.Time) bool {
	date := monthDay(at.UTC())
	start := int(w.StartMonth)*100 + w.StartDay
	end := int(w.EndMonth)*100 + w.EndDay
	if start <= end {
		return date >= start && date <= end
	}
	return date >= start || date <= end
}

type SportConfig struct {
	ID                 Sport
	DisplayName        string
	OddsAPIKey         string
	Season             SeasonWindow
	GamesPerSeason     int
	HomeAdvantage      float64
	MarketAnchors      []float64
	PollCadence        PollCadence
	DefaultKellyRange  KellyRange
	DefaultModelFamily string
}

type SportRegistry struct {
	configs      map[Sport]SportConfig
	oddsAPIIndex map[string]Sport
	displayOrder []Sport
}

func DefaultSportRegistry() SportRegistry {
	configs := []SportConfig{
		{
			ID:                 SportMLB,
			DisplayName:        "Major League Baseball",
			OddsAPIKey:         "baseball_mlb",
			Season:             SeasonWindow{StartMonth: time.March, StartDay: 1, EndMonth: time.November, EndDay: 15},
			GamesPerSeason:     162,
			HomeAdvantage:      0.15,
			MarketAnchors:      []float64{7.5, 8.0, 8.5, 9.0},
			PollCadence:        PollCadence{Pregame: 5 * time.Minute, Live: 60 * time.Second},
			DefaultKellyRange:  KellyRange{Min: 0.15, Max: 0.35},
			DefaultModelFamily: "starter-run-environment",
		},
		{
			ID:                 SportNBA,
			DisplayName:        "National Basketball Association",
			OddsAPIKey:         "basketball_nba",
			Season:             SeasonWindow{StartMonth: time.October, StartDay: 1, EndMonth: time.June, EndDay: 30},
			GamesPerSeason:     82,
			HomeAdvantage:      2.5,
			MarketAnchors:      []float64{3.0, 5.0, 7.0},
			PollCadence:        PollCadence{Pregame: 3 * time.Minute, Live: 45 * time.Second},
			DefaultKellyRange:  KellyRange{Min: 0.1, Max: 0.25},
			DefaultModelFamily: "lineup-adjusted-net-rating",
		},
		{
			ID:                 SportNHL,
			DisplayName:        "National Hockey League",
			OddsAPIKey:         "icehockey_nhl",
			Season:             SeasonWindow{StartMonth: time.October, StartDay: 1, EndMonth: time.June, EndDay: 30},
			GamesPerSeason:     82,
			HomeAdvantage:      0.2,
			MarketAnchors:      []float64{5.5, 6.0, 6.5},
			PollCadence:        PollCadence{Pregame: 4 * time.Minute, Live: 60 * time.Second},
			DefaultKellyRange:  KellyRange{Min: 0.08, Max: 0.2},
			DefaultModelFamily: "xg-goalie-quality",
		},
		{
			ID:                 SportNFL,
			DisplayName:        "National Football League",
			OddsAPIKey:         "americanfootball_nfl",
			Season:             SeasonWindow{StartMonth: time.August, StartDay: 1, EndMonth: time.February, EndDay: 20},
			GamesPerSeason:     17,
			HomeAdvantage:      1.7,
			MarketAnchors:      []float64{3.0, 7.0, 10.0, 14.0},
			PollCadence:        PollCadence{Pregame: 10 * time.Minute, Live: 90 * time.Second},
			DefaultKellyRange:  KellyRange{Min: 0.05, Max: 0.15},
			DefaultModelFamily: "epa-dvoa-situational",
		},
	}

	registry := SportRegistry{
		configs:      make(map[Sport]SportConfig, len(configs)),
		oddsAPIIndex: make(map[string]Sport, len(configs)),
		displayOrder: make([]Sport, 0, len(configs)),
	}
	for _, config := range configs {
		registry.configs[config.ID] = cloneSportConfig(config)
		registry.oddsAPIIndex[config.OddsAPIKey] = config.ID
		registry.displayOrder = append(registry.displayOrder, config.ID)
	}
	return registry
}

func (r SportRegistry) All() []SportConfig {
	items := make([]SportConfig, 0, len(r.displayOrder))
	for _, id := range r.displayOrder {
		if config, ok := r.configs[id]; ok {
			items = append(items, cloneSportConfig(config))
		}
	}
	return items
}

func (r SportRegistry) Get(id Sport) (SportConfig, bool) {
	config, ok := r.configs[id]
	if !ok {
		return SportConfig{}, false
	}
	return cloneSportConfig(config), true
}

func (r SportRegistry) GetByOddsAPIKey(key string) (SportConfig, bool) {
	id, ok := r.oddsAPIIndex[strings.TrimSpace(strings.ToLower(key))]
	if !ok {
		return SportConfig{}, false
	}
	return r.Get(id)
}

func (r SportRegistry) ActiveSports(at time.Time) []SportConfig {
	active := make([]SportConfig, 0, len(r.displayOrder))
	for _, config := range r.All() {
		if config.Season.Contains(at) {
			active = append(active, config)
		}
	}
	return active
}

func (r SportRegistry) ActiveOddsAPISports(at time.Time, allowlist []string) []string {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, sport := range allowlist {
		normalized := strings.TrimSpace(strings.ToLower(sport))
		if normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}

	active := make([]string, 0, len(r.displayOrder))
	for _, config := range r.ActiveSports(at) {
		if len(allowed) > 0 {
			if _, ok := allowed[config.OddsAPIKey]; !ok {
				continue
			}
		}
		active = append(active, config.OddsAPIKey)
	}

	slices.SortFunc(active, func(a string, b string) int {
		return cmp.Compare(a, b)
	})
	return active
}

func cloneSportConfig(config SportConfig) SportConfig {
	config.MarketAnchors = append([]float64(nil), config.MarketAnchors...)
	return config
}

func monthDay(at time.Time) int {
	return int(at.Month())*100 + at.Day()
}
