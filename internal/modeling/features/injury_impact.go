package features

func injuryFeatures(in InjuryInputs) []Feature {
	homeAbsence := 1 - in.HomeAvailability
	awayAbsence := 1 - in.AwayAvailability

	return []Feature{
		{Name: "injury_home_availability", Value: in.HomeAvailability},
		{Name: "injury_away_availability", Value: in.AwayAvailability},
		{Name: "injury_home_absence", Value: homeAbsence},
		{Name: "injury_away_absence", Value: awayAbsence},
		{Name: "injury_availability_edge_norm", Value: clamp((in.HomeAvailability-in.AwayAvailability)*2.0, -1, 1)},
	}
}
