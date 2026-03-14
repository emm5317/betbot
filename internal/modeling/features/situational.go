package features

func situationalFeatures(in SituationalInputs, cfg BuilderConfig) []Feature {
	restDiff := float64(in.HomeRestDays - in.AwayRestDays)
	travelEdge := in.AwayTravelMiles - in.HomeTravelMiles
	densityEdge := float64(in.AwayGamesLast7 - in.HomeGamesLast7)

	neutralSite := 0.0
	if in.NeutralSiteGame {
		neutralSite = 1.0
	}

	return []Feature{
		{Name: "situational_rest_days_diff_norm", Value: clamp(restDiff/4.0, -1, 1)},
		{Name: "situational_travel_edge_norm", Value: clamp(travelEdge/cfg.TravelScale, -1, 1)},
		{Name: "situational_schedule_density_edge_norm", Value: clamp(densityEdge/4.0, -1, 1)},
		{Name: "situational_neutral_site", Value: neutralSite},
	}
}
