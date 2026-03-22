ALTER TABLE recommendation_snapshots
    DROP CONSTRAINT recommendation_snapshots_recommended_side_check,
    ADD CONSTRAINT recommendation_snapshots_recommended_side_check
        CHECK (recommended_side = ANY (ARRAY['home', 'away', 'over', 'under']));
