-- Reverse manual bet tracking alterations.

ALTER TABLE bets DROP COLUMN IF EXISTS user_notes;

ALTER TABLE bets ALTER COLUMN adapter_name DROP DEFAULT;

ALTER TABLE bets DROP CONSTRAINT IF EXISTS bets_recommended_side_check;
ALTER TABLE bets ADD CONSTRAINT bets_recommended_side_check
    CHECK (recommended_side IN ('home', 'away'));

ALTER TABLE bets ADD CONSTRAINT bets_edge_check CHECK (edge >= 0);
ALTER TABLE bets ADD CONSTRAINT bets_market_probability_check CHECK (market_probability > 0 AND market_probability < 1);
ALTER TABLE bets ADD CONSTRAINT bets_model_probability_check CHECK (model_probability > 0 AND model_probability < 1);

ALTER TABLE bets ALTER COLUMN edge SET NOT NULL;
ALTER TABLE bets ALTER COLUMN market_probability SET NOT NULL;
ALTER TABLE bets ALTER COLUMN model_probability SET NOT NULL;
ALTER TABLE bets ALTER COLUMN snapshot_id SET NOT NULL;
