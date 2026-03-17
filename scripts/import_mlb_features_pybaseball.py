#!/usr/bin/env python
"""Import MLB team/pitcher season stats from pybaseball into betbot tables."""

from __future__ import annotations

import argparse
import hashlib
import os
import re
from datetime import date
from typing import Optional

try:
    import psycopg
except ImportError as exc:  # pragma: no cover
    raise SystemExit("psycopg is required; install with `python -m pip install psycopg[binary]`") from exc

try:
    from pybaseball import batting_stats, pitching_stats
except ImportError as exc:  # pragma: no cover
    raise SystemExit("pybaseball is required; install with `python -m pip install pybaseball`") from exc


TEAM_NAME_MAP = {
    "ARI": "Arizona Diamondbacks",
    "ATL": "Atlanta Braves",
    "BAL": "Baltimore Orioles",
    "BOS": "Boston Red Sox",
    "CHC": "Chicago Cubs",
    "CHW": "Chicago White Sox",
    "CIN": "Cincinnati Reds",
    "CLE": "Cleveland Guardians",
    "COL": "Colorado Rockies",
    "DET": "Detroit Tigers",
    "HOU": "Houston Astros",
    "KC": "Kansas City Royals",
    "LAA": "Los Angeles Angels",
    "LAD": "Los Angeles Dodgers",
    "MIA": "Miami Marlins",
    "MIL": "Milwaukee Brewers",
    "MIN": "Minnesota Twins",
    "NYM": "New York Mets",
    "NYY": "New York Yankees",
    "OAK": "Athletics",
    "PHI": "Philadelphia Phillies",
    "PIT": "Pittsburgh Pirates",
    "SD": "San Diego Padres",
    "SEA": "Seattle Mariners",
    "SF": "San Francisco Giants",
    "STL": "St. Louis Cardinals",
    "TB": "Tampa Bay Rays",
    "TEX": "Texas Rangers",
    "TOR": "Toronto Blue Jays",
    "WSH": "Washington Nationals",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Import pybaseball MLB season features")
    parser.add_argument("--season", type=int, required=True, help="MLB season year (e.g. 2025)")
    parser.add_argument(
        "--database-url",
        default=os.getenv("BETBOT_DATABASE_URL", ""),
        help="Postgres connection string (defaults to BETBOT_DATABASE_URL)",
    )
    parser.add_argument("--dry-run", action="store_true", help="Fetch and validate only")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if not args.database_url and not args.dry_run:
        raise SystemExit("database URL required via --database-url or BETBOT_DATABASE_URL")

    stat_date = date(args.season, 12, 31)
    source = "pybaseball"

    print(f"fetching pybaseball batting/pitching stats for season {args.season}")
    batting = batting_stats(args.season, qual=0)
    pitching = pitching_stats(args.season, qual=0)
    print(f"batting rows: {len(batting)}")
    print(f"pitching rows: {len(pitching)}")

    if args.dry_run:
        print("dry-run enabled; no writes performed")
        return

    team_upserts = 0
    pitcher_upserts = 0

    team_pitching = {}
    for _, row in pitching.iterrows():
        team = str(row.get("Team") or "").strip().upper()
        if not team:
            continue
        if team not in team_pitching:
            team_pitching[team] = {"RA": 0.0, "ERA": None}
        if team_pitching[team]["ERA"] is None:
            team_pitching[team]["ERA"] = to_float(row.get("ERA"))
        # `R` is runs allowed from pitcher perspective in pybaseball tables.
        runs_allowed = to_float(row.get("R"))
        if runs_allowed is not None:
            team_pitching[team]["RA"] += runs_allowed

    with psycopg.connect(args.database_url) as conn:
        with conn.cursor() as cur:
            for _, row in batting.iterrows():
                team_abbr = str(row.get("Team") or "").strip().upper()
                if not team_abbr:
                    continue
                team_name = TEAM_NAME_MAP.get(team_abbr, team_abbr)
                games = int(to_float(row.get("G")) or 0)
                wins = int(to_float(row.get("W")) or 0)
                losses = int(to_float(row.get("L")) or 0)
                runs_scored = int(to_float(row.get("R")) or 0)
                runs_allowed = int(team_pitching.get(team_abbr, {}).get("RA") or 0)
                ops = to_float(row.get("OPS"))
                era = team_pitching.get(team_abbr, {}).get("ERA")

                cur.execute(
                    """
                    INSERT INTO mlb_team_stats (
                        source, external_id, season, season_type, stat_date, team_name,
                        games_played, wins, losses, runs_scored, runs_allowed, batting_ops, team_era
                    ) VALUES (%s, %s, %s, 'regular', %s, %s, %s, %s, %s, %s, %s, %s, %s)
                    ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
                    SET
                        team_name = EXCLUDED.team_name,
                        games_played = EXCLUDED.games_played,
                        wins = EXCLUDED.wins,
                        losses = EXCLUDED.losses,
                        runs_scored = EXCLUDED.runs_scored,
                        runs_allowed = EXCLUDED.runs_allowed,
                        batting_ops = EXCLUDED.batting_ops,
                        team_era = EXCLUDED.team_era,
                        updated_at = NOW()
                    """,
                    (
                        source,
                        team_abbr.lower(),
                        args.season,
                        stat_date,
                        team_name,
                        games,
                        wins,
                        losses,
                        runs_scored,
                        runs_allowed,
                        ops,
                        era,
                    ),
                )
                team_upserts += 1

            for _, row in pitching.iterrows():
                player_name = str(row.get("Name") or "").strip()
                team_abbr = str(row.get("Team") or "").strip().upper()
                if not player_name or not team_abbr:
                    continue

                games_started = int(to_float(row.get("GS")) or 0)
                if games_started <= 0:
                    continue

                external_id = pitcher_external_id(args.season, team_abbr, player_name)
                team_name = TEAM_NAME_MAP.get(team_abbr, team_abbr)
                innings_pitched = to_float(row.get("IP"))
                era = to_float(row.get("ERA"))
                fip = to_float(row.get("FIP"))
                whip = to_float(row.get("WHIP"))
                strikeout_rate = parse_rate(row.get("K%"))
                walk_rate = parse_rate(row.get("BB%"))

                cur.execute(
                    """
                    INSERT INTO mlb_pitcher_stats (
                        source, external_id, season, season_type, stat_date, player_name,
                        team_external_id, team_name, games_started, innings_pitched,
                        era, fip, whip, strikeout_rate, walk_rate
                    ) VALUES (
                        %s, %s, %s, 'regular', %s, %s,
                        %s, %s, %s, %s,
                        %s, %s, %s, %s, %s
                    )
                    ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
                    SET
                        player_name = EXCLUDED.player_name,
                        team_external_id = EXCLUDED.team_external_id,
                        team_name = EXCLUDED.team_name,
                        games_started = EXCLUDED.games_started,
                        innings_pitched = EXCLUDED.innings_pitched,
                        era = EXCLUDED.era,
                        fip = EXCLUDED.fip,
                        whip = EXCLUDED.whip,
                        strikeout_rate = EXCLUDED.strikeout_rate,
                        walk_rate = EXCLUDED.walk_rate,
                        updated_at = NOW()
                    """,
                    (
                        source,
                        external_id,
                        args.season,
                        stat_date,
                        player_name,
                        team_abbr.lower(),
                        team_name,
                        games_started,
                        innings_pitched,
                        era,
                        fip,
                        whip,
                        strikeout_rate,
                        walk_rate,
                    ),
                )
                pitcher_upserts += 1
        conn.commit()

    print(f"mlb_team_stats_upserts: {team_upserts}")
    print(f"mlb_pitcher_stats_upserts: {pitcher_upserts}")


def to_float(value: object) -> Optional[float]:
    if value is None:
        return None
    text = str(value).strip()
    if text == "" or text == "nan":
        return None
    try:
        return float(text)
    except ValueError:
        return None


def parse_rate(value: object) -> Optional[float]:
    if value is None:
        return None
    text = str(value).strip()
    if text == "" or text.lower() == "nan":
        return None
    if text.endswith("%"):
        text = text[:-1]
    try:
        parsed = float(text)
    except ValueError:
        return None
    if parsed > 1:
        parsed = parsed / 100.0
    if parsed < 0:
        return None
    return parsed


def pitcher_external_id(season: int, team_abbr: str, player_name: str) -> str:
    normalized = re.sub(r"\s+", " ", player_name).strip().lower()
    raw = f"{season}|{team_abbr}|{normalized}"
    digest = hashlib.sha256(raw.encode("utf-8")).hexdigest()[:16]
    return f"pyb-{digest}"


if __name__ == "__main__":
    main()
