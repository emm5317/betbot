SELECT ensure_odds_history_partition((date_trunc('month', NOW()) + INTERVAL '2 month')::DATE);
SELECT ensure_odds_history_partition((date_trunc('month', NOW()) + INTERVAL '3 month')::DATE);
