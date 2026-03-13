CREATE TABLE game_weather_snapshots (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    forecast_time TIMESTAMPTZ NOT NULL,
    venue_name TEXT NOT NULL,
    venue_timezone TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    roof_type TEXT NOT NULL,
    weather_code INTEGER,
    temperature_f DOUBLE PRECISION,
    apparent_temperature_f DOUBLE PRECISION,
    precipitation_probability DOUBLE PRECISION,
    precipitation_inches DOUBLE PRECISION,
    wind_speed_mph DOUBLE PRECISION,
    wind_gust_mph DOUBLE PRECISION,
    wind_direction_degrees INTEGER,
    raw_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (game_id, source)
);

CREATE INDEX game_weather_snapshots_forecast_time_idx
    ON game_weather_snapshots (forecast_time DESC);
