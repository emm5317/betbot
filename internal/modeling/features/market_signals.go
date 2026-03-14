package features

import "math"

func marketPriorFeatures(market MarketInputs, cfg BuilderConfig, anchors []float64) []Feature {
	return []Feature{
		{Name: "market_home_implied_prob", Value: market.HomeMoneylineProbability},
		{Name: "market_away_implied_prob", Value: market.AwayMoneylineProbability},
		{Name: "market_home_spread", Value: market.HomeSpread},
		{Name: "market_home_spread_norm", Value: clamp(market.HomeSpread/cfg.SpreadScale, -1, 1)},
		{Name: "market_total_points", Value: market.TotalPoints},
		{Name: "market_total_points_norm", Value: clamp((market.TotalPoints-cfg.TotalBaseline)/cfg.TotalScale, -1, 1)},
		{Name: "market_key_number_proximity", Value: keyNumberProximity(math.Abs(market.HomeSpread), anchors, cfg.KeyNumberScale)},
	}
}

func keyNumberProximity(absSpread float64, anchors []float64, scale float64) float64 {
	if len(anchors) == 0 {
		return 0
	}
	closest := math.MaxFloat64
	for _, anchor := range anchors {
		distance := math.Abs(absSpread - anchor)
		if distance < closest {
			closest = distance
		}
	}
	return 1 - clamp(closest/scale, 0, 1)
}
