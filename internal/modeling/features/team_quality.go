package features

func teamQualityFeatures(team TeamQualityInputs, cfg BuilderConfig) []Feature {
	homeNet := team.HomeOffenseRating - team.HomeDefenseRating
	awayNet := team.AwayOffenseRating - team.AwayDefenseRating

	return []Feature{
		{Name: "team_quality_power_rating_diff_norm", Value: clamp((team.HomePowerRating-team.AwayPowerRating)/cfg.RatingScale, -1, 1)},
		{Name: "team_quality_offense_diff_norm", Value: clamp((team.HomeOffenseRating-team.AwayOffenseRating)/cfg.RatingScale, -1, 1)},
		{Name: "team_quality_defense_diff_norm", Value: clamp((team.AwayDefenseRating-team.HomeDefenseRating)/cfg.RatingScale, -1, 1)},
		{Name: "team_quality_net_rating_diff_norm", Value: clamp((homeNet-awayNet)/cfg.RatingScale, -1, 1)},
	}
}
