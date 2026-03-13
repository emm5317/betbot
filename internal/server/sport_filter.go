package server

import (
	"fmt"
	"net/url"
	"strings"
)

type sportFilterDefinition struct {
	Key     string
	DBSport string
	Label   string
}

type sportFilterSelection struct {
	Key     string
	DBSport string
	Label   string
}

type sportFilterOption struct {
	Value    string
	Label    string
	Selected bool
}

var supportedSportFilters = []sportFilterDefinition{
	{Key: "baseball_mlb", DBSport: "MLB", Label: "MLB"},
	{Key: "basketball_nba", DBSport: "NBA", Label: "NBA"},
	{Key: "icehockey_nhl", DBSport: "NHL", Label: "NHL"},
	{Key: "americanfootball_nfl", DBSport: "NFL", Label: "NFL"},
}

var supportedSportFilterByKey = map[string]sportFilterDefinition{
	"baseball_mlb":         {Key: "baseball_mlb", DBSport: "MLB", Label: "MLB"},
	"basketball_nba":       {Key: "basketball_nba", DBSport: "NBA", Label: "NBA"},
	"icehockey_nhl":        {Key: "icehockey_nhl", DBSport: "NHL", Label: "NHL"},
	"americanfootball_nfl": {Key: "americanfootball_nfl", DBSport: "NFL", Label: "NFL"},
}

const supportedSportFilterList = "baseball_mlb, basketball_nba, icehockey_nhl, americanfootball_nfl"

func resolveSportFilter(raw string) (sportFilterSelection, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return sportFilterSelection{Label: "All sports"}, nil
	}

	definition, ok := supportedSportFilterByKey[normalized]
	if !ok {
		return sportFilterSelection{Label: "All sports"}, fmt.Errorf("invalid sport filter %q; allowed values: %s", raw, supportedSportFilterList)
	}
	return sportFilterSelection{
		Key:     definition.Key,
		DBSport: definition.DBSport,
		Label:   definition.Label,
	}, nil
}

func (s sportFilterSelection) querySuffix() string {
	if s.Key == "" {
		return ""
	}
	return "?sport=" + url.QueryEscape(s.Key)
}

func (s sportFilterSelection) storeParam() *string {
	if s.DBSport == "" {
		return nil
	}
	sport := s.DBSport
	return &sport
}

func applySportFilterView(view map[string]any, actionPath string, selected sportFilterSelection) {
	view["SportFilterAction"] = actionPath
	view["SportFilterOptions"] = sportFilterOptions(selected.Key)
	view["SelectedSport"] = selected.Key
	view["SelectedSportLabel"] = selected.Label
	view["SelectedSportQuery"] = selected.querySuffix()
}

func sportFilterOptions(selectedKey string) []sportFilterOption {
	options := make([]sportFilterOption, 0, len(supportedSportFilters)+1)
	options = append(options, sportFilterOption{
		Value:    "",
		Label:    "All sports",
		Selected: selectedKey == "",
	})

	for _, definition := range supportedSportFilters {
		options = append(options, sportFilterOption{
			Value:    definition.Key,
			Label:    fmt.Sprintf("%s (%s)", definition.Label, definition.Key),
			Selected: definition.Key == selectedKey,
		})
	}
	return options
}
