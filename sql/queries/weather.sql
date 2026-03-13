-- name: ListUpcomingWeatherGames :many
SELECT id, source, external_id, sport, home_team, away_team, commence_time, created_at, updated_at
FROM games
WHERE sport = ANY(sqlc.arg(sports)::text[])
  AND commence_time >= sqlc.arg(window_start)
  AND commence_time < sqlc.arg(window_end)
ORDER BY commence_time ASC;

-- name: UpsertGameWeatherSnapshot :exec
INSERT INTO game_weather_snapshots (
    game_id,
    source,
    forecast_time,
    venue_name,
    venue_timezone,
    latitude,
    longitude,
    roof_type,
    weather_code,
    temperature_f,
    apparent_temperature_f,
    precipitation_probability,
    precipitation_inches,
    wind_speed_mph,
    wind_gust_mph,
    wind_direction_degrees,
    raw_json
) VALUES (
    sqlc.arg(game_id),
    sqlc.arg(source),
    sqlc.arg(forecast_time),
    sqlc.arg(venue_name),
    sqlc.arg(venue_timezone),
    sqlc.arg(latitude),
    sqlc.arg(longitude),
    sqlc.arg(roof_type),
    sqlc.arg(weather_code),
    sqlc.arg(temperature_f),
    sqlc.arg(apparent_temperature_f),
    sqlc.arg(precipitation_probability),
    sqlc.arg(precipitation_inches),
    sqlc.arg(wind_speed_mph),
    sqlc.arg(wind_gust_mph),
    sqlc.arg(wind_direction_degrees),
    sqlc.arg(raw_json)
)
ON CONFLICT (game_id, source) DO UPDATE
SET
    forecast_time = EXCLUDED.forecast_time,
    venue_name = EXCLUDED.venue_name,
    venue_timezone = EXCLUDED.venue_timezone,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    roof_type = EXCLUDED.roof_type,
    weather_code = EXCLUDED.weather_code,
    temperature_f = EXCLUDED.temperature_f,
    apparent_temperature_f = EXCLUDED.apparent_temperature_f,
    precipitation_probability = EXCLUDED.precipitation_probability,
    precipitation_inches = EXCLUDED.precipitation_inches,
    wind_speed_mph = EXCLUDED.wind_speed_mph,
    wind_gust_mph = EXCLUDED.wind_gust_mph,
    wind_direction_degrees = EXCLUDED.wind_direction_degrees,
    raw_json = EXCLUDED.raw_json,
    updated_at = NOW()
WHERE
    game_weather_snapshots.forecast_time IS DISTINCT FROM EXCLUDED.forecast_time
    OR game_weather_snapshots.venue_name IS DISTINCT FROM EXCLUDED.venue_name
    OR game_weather_snapshots.venue_timezone IS DISTINCT FROM EXCLUDED.venue_timezone
    OR game_weather_snapshots.latitude IS DISTINCT FROM EXCLUDED.latitude
    OR game_weather_snapshots.longitude IS DISTINCT FROM EXCLUDED.longitude
    OR game_weather_snapshots.roof_type IS DISTINCT FROM EXCLUDED.roof_type
    OR game_weather_snapshots.weather_code IS DISTINCT FROM EXCLUDED.weather_code
    OR game_weather_snapshots.temperature_f IS DISTINCT FROM EXCLUDED.temperature_f
    OR game_weather_snapshots.apparent_temperature_f IS DISTINCT FROM EXCLUDED.apparent_temperature_f
    OR game_weather_snapshots.precipitation_probability IS DISTINCT FROM EXCLUDED.precipitation_probability
    OR game_weather_snapshots.precipitation_inches IS DISTINCT FROM EXCLUDED.precipitation_inches
    OR game_weather_snapshots.wind_speed_mph IS DISTINCT FROM EXCLUDED.wind_speed_mph
    OR game_weather_snapshots.wind_gust_mph IS DISTINCT FROM EXCLUDED.wind_gust_mph
    OR game_weather_snapshots.wind_direction_degrees IS DISTINCT FROM EXCLUDED.wind_direction_degrees
    OR game_weather_snapshots.raw_json IS DISTINCT FROM EXCLUDED.raw_json;
