package features

import "math"

func weatherFeatures(in WeatherInputs, cfg BuilderConfig) []Feature {
	if in.IsDome {
		return []Feature{
			{Name: "weather_is_dome", Value: 1},
			{Name: "weather_temp_penalty_norm", Value: 0},
			{Name: "weather_wind_penalty_norm", Value: 0},
			{Name: "weather_precip_penalty_norm", Value: 0},
			{Name: "weather_severity_norm", Value: 0},
		}
	}

	tempPenalty := clamp(math.Abs(in.TemperatureF-70.0)/cfg.TemperatureScale, 0, 1)
	windPenalty := clamp(in.WindMPH/cfg.WindScale, 0, 1)
	precipPenalty := clamp(in.PrecipitationMM/cfg.PrecipitationScale, 0, 1)
	severity := clamp(tempPenalty*0.35+windPenalty*0.40+precipPenalty*0.25, 0, 1)

	return []Feature{
		{Name: "weather_is_dome", Value: 0},
		{Name: "weather_temp_penalty_norm", Value: tempPenalty},
		{Name: "weather_wind_penalty_norm", Value: windPenalty},
		{Name: "weather_precip_penalty_norm", Value: precipPenalty},
		{Name: "weather_severity_norm", Value: severity},
	}
}
