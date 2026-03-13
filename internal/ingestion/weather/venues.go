package weather

import "time"

type RoofType string

const (
	RoofTypeOutdoor     RoofType = "outdoor"
	RoofTypeDome        RoofType = "dome"
	RoofTypeRetractable RoofType = "retractable"
)

type RoofWeatherPolicy string

const (
	RoofWeatherPolicyOutdoor            RoofWeatherPolicy = "outdoor-fetch"
	RoofWeatherPolicyFixedIndoor        RoofWeatherPolicy = "fixed-indoor"
	RoofWeatherPolicyRetractableUnknown RoofWeatherPolicy = "retractable-unknown"
)

type Venue struct {
	Name      string
	Timezone  string
	Latitude  float64
	Longitude float64
	RoofType  RoofType
}

func (v Venue) IsOutdoor() bool {
	return v.RoofType == RoofTypeOutdoor
}

func (v Venue) WeatherPolicy() RoofWeatherPolicy {
	switch v.RoofType {
	case RoofTypeOutdoor:
		return RoofWeatherPolicyOutdoor
	case RoofTypeDome:
		return RoofWeatherPolicyFixedIndoor
	case RoofTypeRetractable:
		return RoofWeatherPolicyRetractableUnknown
	default:
		return RoofWeatherPolicyFixedIndoor
	}
}

func (p RoofWeatherPolicy) RequiresProvider() bool {
	return p == RoofWeatherPolicyOutdoor
}

func (p RoofWeatherPolicy) IndoorReason() string {
	switch p {
	case RoofWeatherPolicyFixedIndoor:
		return "fixed-roof-indoor"
	case RoofWeatherPolicyRetractableUnknown:
		return "retractable-roof-state-unknown"
	default:
		return "provider-not-required"
	}
}

type venueOverride struct {
	Start time.Time
	End   time.Time
	Venue Venue
}

type venueEntry struct {
	Default   Venue
	Overrides []venueOverride
}

func LookupVenue(sport string, homeTeam string, commenceTime time.Time) (Venue, bool) {
	key := venueKey(normalizeSport(sport), homeTeam)
	entry, ok := supportedVenues[key]
	if !ok {
		return Venue{}, false
	}

	at := commenceTime.UTC()
	for _, override := range entry.Overrides {
		if !at.Before(override.Start) && at.Before(override.End) {
			return override.Venue, true
		}
	}
	return entry.Default, true
}

func venueKey(sport string, teamName string) string {
	return sport + ":" + normalizeLabel(teamName)
}

var supportedVenues = map[string]venueEntry{
	venueKey("MLB", "Arizona Diamondbacks"): {Default: Venue{Name: "Chase Field", Timezone: "America/Phoenix", Latitude: 33.4453, Longitude: -112.0667, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Athletics"): {
		Default:   Venue{Name: "Sutter Health Park", Timezone: "America/Los_Angeles", Latitude: 38.5806, Longitude: -121.5138, RoofType: RoofTypeOutdoor},
		Overrides: athleticsTemporaryHomeOverrides(),
	},
	venueKey("MLB", "Atlanta Braves"):        {Default: Venue{Name: "Truist Park", Timezone: "America/New_York", Latitude: 33.8907, Longitude: -84.4677, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Baltimore Orioles"):     {Default: Venue{Name: "Oriole Park at Camden Yards", Timezone: "America/New_York", Latitude: 39.2840, Longitude: -76.6217, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Boston Red Sox"):        {Default: Venue{Name: "Fenway Park", Timezone: "America/New_York", Latitude: 42.3467, Longitude: -71.0972, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Chicago Cubs"):          {Default: Venue{Name: "Wrigley Field", Timezone: "America/Chicago", Latitude: 41.9484, Longitude: -87.6553, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Chicago White Sox"):     {Default: Venue{Name: "Rate Field", Timezone: "America/Chicago", Latitude: 41.8300, Longitude: -87.6338, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Cincinnati Reds"):       {Default: Venue{Name: "Great American Ball Park", Timezone: "America/New_York", Latitude: 39.0979, Longitude: -84.5081, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Cleveland Guardians"):   {Default: Venue{Name: "Progressive Field", Timezone: "America/New_York", Latitude: 41.4962, Longitude: -81.6852, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Colorado Rockies"):      {Default: Venue{Name: "Coors Field", Timezone: "America/Denver", Latitude: 39.7559, Longitude: -104.9942, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Detroit Tigers"):        {Default: Venue{Name: "Comerica Park", Timezone: "America/New_York", Latitude: 42.3390, Longitude: -83.0485, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Houston Astros"):        {Default: Venue{Name: "Daikin Park", Timezone: "America/Chicago", Latitude: 29.7573, Longitude: -95.3555, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Kansas City Royals"):    {Default: Venue{Name: "Kauffman Stadium", Timezone: "America/Chicago", Latitude: 39.0517, Longitude: -94.4803, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Los Angeles Angels"):    {Default: Venue{Name: "Angel Stadium", Timezone: "America/Los_Angeles", Latitude: 33.8003, Longitude: -117.8827, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Los Angeles Dodgers"):   {Default: Venue{Name: "Dodger Stadium", Timezone: "America/Los_Angeles", Latitude: 34.0739, Longitude: -118.2400, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Miami Marlins"):         {Default: Venue{Name: "loanDepot park", Timezone: "America/New_York", Latitude: 25.7781, Longitude: -80.2197, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Milwaukee Brewers"):     {Default: Venue{Name: "American Family Field", Timezone: "America/Chicago", Latitude: 43.0280, Longitude: -87.9712, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Minnesota Twins"):       {Default: Venue{Name: "Target Field", Timezone: "America/Chicago", Latitude: 44.9817, Longitude: -93.2775, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "New York Mets"):         {Default: Venue{Name: "Citi Field", Timezone: "America/New_York", Latitude: 40.7571, Longitude: -73.8458, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "New York Yankees"):      {Default: Venue{Name: "Yankee Stadium", Timezone: "America/New_York", Latitude: 40.8296, Longitude: -73.9262, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Philadelphia Phillies"): {Default: Venue{Name: "Citizens Bank Park", Timezone: "America/New_York", Latitude: 39.9061, Longitude: -75.1665, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Pittsburgh Pirates"):    {Default: Venue{Name: "PNC Park", Timezone: "America/New_York", Latitude: 40.4469, Longitude: -80.0057, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "San Diego Padres"):      {Default: Venue{Name: "Petco Park", Timezone: "America/Los_Angeles", Latitude: 32.7073, Longitude: -117.1566, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "San Francisco Giants"):  {Default: Venue{Name: "Oracle Park", Timezone: "America/Los_Angeles", Latitude: 37.7786, Longitude: -122.3893, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Seattle Mariners"):      {Default: Venue{Name: "T-Mobile Park", Timezone: "America/Los_Angeles", Latitude: 47.5914, Longitude: -122.3325, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "St. Louis Cardinals"):   {Default: Venue{Name: "Busch Stadium", Timezone: "America/Chicago", Latitude: 38.6226, Longitude: -90.1928, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Tampa Bay Rays"):        {Default: Venue{Name: "Tropicana Field", Timezone: "America/New_York", Latitude: 27.7683, Longitude: -82.6534, RoofType: RoofTypeDome}},
	venueKey("MLB", "Texas Rangers"):         {Default: Venue{Name: "Globe Life Field", Timezone: "America/Chicago", Latitude: 32.7513, Longitude: -97.0825, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Toronto Blue Jays"):     {Default: Venue{Name: "Rogers Centre", Timezone: "America/Toronto", Latitude: 43.6414, Longitude: -79.3894, RoofType: RoofTypeRetractable}},
	venueKey("MLB", "Washington Nationals"):  {Default: Venue{Name: "Nationals Park", Timezone: "America/New_York", Latitude: 38.8730, Longitude: -77.0074, RoofType: RoofTypeOutdoor}},
	venueKey("MLB", "Oakland Athletics"): {
		Default:   Venue{Name: "Sutter Health Park", Timezone: "America/Los_Angeles", Latitude: 38.5806, Longitude: -121.5138, RoofType: RoofTypeOutdoor},
		Overrides: athleticsTemporaryHomeOverrides(),
	},
	venueKey("NFL", "Arizona Cardinals"):     {Default: Venue{Name: "State Farm Stadium", Timezone: "America/Phoenix", Latitude: 33.5276, Longitude: -112.2626, RoofType: RoofTypeRetractable}},
	venueKey("NFL", "Atlanta Falcons"):       {Default: Venue{Name: "Mercedes-Benz Stadium", Timezone: "America/New_York", Latitude: 33.7554, Longitude: -84.4009, RoofType: RoofTypeDome}},
	venueKey("NFL", "Baltimore Ravens"):      {Default: Venue{Name: "M&T Bank Stadium", Timezone: "America/New_York", Latitude: 39.2779, Longitude: -76.6227, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Buffalo Bills"):         {Default: Venue{Name: "Highmark Stadium", Timezone: "America/New_York", Latitude: 42.7738, Longitude: -78.7868, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Carolina Panthers"):     {Default: Venue{Name: "Bank of America Stadium", Timezone: "America/New_York", Latitude: 35.2258, Longitude: -80.8528, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Chicago Bears"):         {Default: Venue{Name: "Soldier Field", Timezone: "America/Chicago", Latitude: 41.8623, Longitude: -87.6167, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Cincinnati Bengals"):    {Default: Venue{Name: "Paycor Stadium", Timezone: "America/New_York", Latitude: 39.0955, Longitude: -84.5160, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Cleveland Browns"):      {Default: Venue{Name: "Huntington Bank Field", Timezone: "America/New_York", Latitude: 41.5061, Longitude: -81.6995, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Dallas Cowboys"):        {Default: Venue{Name: "AT&T Stadium", Timezone: "America/Chicago", Latitude: 32.7473, Longitude: -97.0945, RoofType: RoofTypeRetractable}},
	venueKey("NFL", "Denver Broncos"):        {Default: Venue{Name: "Empower Field at Mile High", Timezone: "America/Denver", Latitude: 39.7439, Longitude: -105.0201, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Detroit Lions"):         {Default: Venue{Name: "Ford Field", Timezone: "America/New_York", Latitude: 42.3400, Longitude: -83.0456, RoofType: RoofTypeDome}},
	venueKey("NFL", "Green Bay Packers"):     {Default: Venue{Name: "Lambeau Field", Timezone: "America/Chicago", Latitude: 44.5013, Longitude: -88.0622, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Houston Texans"):        {Default: Venue{Name: "NRG Stadium", Timezone: "America/Chicago", Latitude: 29.6847, Longitude: -95.4107, RoofType: RoofTypeRetractable}},
	venueKey("NFL", "Indianapolis Colts"):    {Default: Venue{Name: "Lucas Oil Stadium", Timezone: "America/Indiana/Indianapolis", Latitude: 39.7601, Longitude: -86.1639, RoofType: RoofTypeRetractable}},
	venueKey("NFL", "Jacksonville Jaguars"):  {Default: Venue{Name: "EverBank Stadium", Timezone: "America/New_York", Latitude: 30.3239, Longitude: -81.6373, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Kansas City Chiefs"):    {Default: Venue{Name: "GEHA Field at Arrowhead Stadium", Timezone: "America/Chicago", Latitude: 39.0489, Longitude: -94.4839, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Las Vegas Raiders"):     {Default: Venue{Name: "Allegiant Stadium", Timezone: "America/Los_Angeles", Latitude: 36.0908, Longitude: -115.1837, RoofType: RoofTypeDome}},
	venueKey("NFL", "Los Angeles Chargers"):  {Default: Venue{Name: "SoFi Stadium", Timezone: "America/Los_Angeles", Latitude: 33.9535, Longitude: -118.3392, RoofType: RoofTypeDome}},
	venueKey("NFL", "Los Angeles Rams"):      {Default: Venue{Name: "SoFi Stadium", Timezone: "America/Los_Angeles", Latitude: 33.9535, Longitude: -118.3392, RoofType: RoofTypeDome}},
	venueKey("NFL", "Miami Dolphins"):        {Default: Venue{Name: "Hard Rock Stadium", Timezone: "America/New_York", Latitude: 25.9580, Longitude: -80.2389, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Minnesota Vikings"):     {Default: Venue{Name: "U.S. Bank Stadium", Timezone: "America/Chicago", Latitude: 44.9737, Longitude: -93.2576, RoofType: RoofTypeDome}},
	venueKey("NFL", "New England Patriots"):  {Default: Venue{Name: "Gillette Stadium", Timezone: "America/New_York", Latitude: 42.0909, Longitude: -71.2643, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "New Orleans Saints"):    {Default: Venue{Name: "Caesars Superdome", Timezone: "America/Chicago", Latitude: 29.9511, Longitude: -90.0812, RoofType: RoofTypeDome}},
	venueKey("NFL", "New York Giants"):       {Default: Venue{Name: "MetLife Stadium", Timezone: "America/New_York", Latitude: 40.8135, Longitude: -74.0745, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "New York Jets"):         {Default: Venue{Name: "MetLife Stadium", Timezone: "America/New_York", Latitude: 40.8135, Longitude: -74.0745, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Philadelphia Eagles"):   {Default: Venue{Name: "Lincoln Financial Field", Timezone: "America/New_York", Latitude: 39.9008, Longitude: -75.1675, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Pittsburgh Steelers"):   {Default: Venue{Name: "Acrisure Stadium", Timezone: "America/New_York", Latitude: 40.4468, Longitude: -80.0158, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "San Francisco 49ers"):   {Default: Venue{Name: "Levi's Stadium", Timezone: "America/Los_Angeles", Latitude: 37.4030, Longitude: -121.9700, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Seattle Seahawks"):      {Default: Venue{Name: "Lumen Field", Timezone: "America/Los_Angeles", Latitude: 47.5952, Longitude: -122.3316, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Tampa Bay Buccaneers"):  {Default: Venue{Name: "Raymond James Stadium", Timezone: "America/New_York", Latitude: 27.9759, Longitude: -82.5033, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Tennessee Titans"):      {Default: Venue{Name: "Nissan Stadium", Timezone: "America/Chicago", Latitude: 36.1665, Longitude: -86.7713, RoofType: RoofTypeOutdoor}},
	venueKey("NFL", "Washington Commanders"): {Default: Venue{Name: "Northwest Stadium", Timezone: "America/New_York", Latitude: 38.9078, Longitude: -76.8645, RoofType: RoofTypeOutdoor}},
}

func athleticsTemporaryHomeOverrides() []venueOverride {
	return []venueOverride{{
		Start: time.Date(2026, time.June, 8, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC),
		Venue: Venue{Name: "Las Vegas Ballpark", Timezone: "America/Los_Angeles", Latitude: 36.0908, Longitude: -115.1834, RoofType: RoofTypeOutdoor},
	}}
}
