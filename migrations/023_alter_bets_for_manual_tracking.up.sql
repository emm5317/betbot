-- Allow manual bet entry: relax constraints that assume model-driven placement.

-- snapshot_id may be NULL for manual bets not linked to a recommendation.
ALTER TABLE bets ALTER COLUMN snapshot_id DROP NOT NULL;

-- model_probability, market_probability, edge may be NULL for manual bets.
ALTER TABLE bets ALTER COLUMN model_probability DROP NOT NULL;
ALTER TABLE bets ALTER COLUMN market_probability DROP NOT NULL;
ALTER TABLE bets ALTER COLUMN edge DROP NOT NULL;

-- Drop CHECK constraints that block manual entry values.
ALTER TABLE bets DROP CONSTRAINT IF EXISTS bets_model_probability_check;
ALTER TABLE bets DROP CONSTRAINT IF EXISTS bets_market_probability_check;
ALTER TABLE bets DROP CONSTRAINT IF EXISTS bets_edge_check;
ALTER TABLE bets DROP CONSTRAINT IF EXISTS bets_recommended_side_check;

-- Re-add recommended_side CHECK with over/under support.
ALTER TABLE bets ADD CONSTRAINT bets_recommended_side_check
    CHECK (recommended_side IN ('home', 'away', 'over', 'under'));

-- Default adapter_name to 'manual' for manual entry.
ALTER TABLE bets ALTER COLUMN adapter_name SET DEFAULT 'manual';

-- Add user notes column for manual bet context.
ALTER TABLE bets ADD COLUMN user_notes TEXT;
