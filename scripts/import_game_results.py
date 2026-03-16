#!/usr/bin/env python
"""Import MLB game outcomes using MLB-StatsAPI into game_results."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from datetime import date, datetime, timedelta, timezone
from typing import Any, Dict, Iterable, Optional

try:
    import psycopg
except ImportError as exc:  # pragma: no cover
    raise SystemExit("psycopg is required; install with `python -m pip install psycopg[binary]`") from exc

try:
    import statsapi  # from MLB-StatsAPI package
except ImportError as exc:  # pragma: no cover
    raise SystemExit("MLB-StatsAPI is required; install with `python -m pip install MLB-StatsAPI`") from exc


RESULTS_SOURCE = "mlb-statsapi"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Import MLB final scores into game_results")
    parser.add_argument(
        "--season",
        type=int,
        help="Season year (e.g. 2025). If provided, auto-derives date range Mar 1..Nov 30.",
    )
    parser.add_argument("--start-date", help="YYYY-MM-DD (overrides season start)")
    parser.add_argument("--end-date", help="YYYY-MM-DD (overrides season end)")
    parser.add_argument(
        "--database-url",
        default=os.getenv("BETBOT_DATABASE_URL", ""),
        help="Postgres connection string (defaults to BETBOT_DATABASE_URL)",
    )
    parser.add_argument("--dry-run", action="store_true", help="Fetch and match only; no writes")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    start_date, end_date = resolve_date_range(args.season, args.start_date, args.end_date)
    print(f"importing game results from {start_date} to {end_date}")

    schedule_rows = list(fetch_schedule(start_date, end_date))
    print(f"schedule rows fetched: {len(schedule_rows)}")

    if not args.database_url and not args.dry_run:
        raise SystemExit("database URL required via --database-url or BETBOT_DATABASE_URL")

    matched = 0
    inserted = 0
    skipped_unmatched = 0
    skipped_nonfinal = 0

    if args.dry_run:
        for game in schedule_rows:
            if not is_final_status(game):
                skipped_nonfinal += 1
        print("dry-run enabled; no writes performed")
        print(f"non_final_skipped: {skipped_nonfinal}")
        return

    with psycopg.connect(args.database_url) as conn:
        with conn.cursor() as cur:
            for game in schedule_rows:
                if not is_final_status(game):
                    skipped_nonfinal += 1
                    continue

                game_id = find_game_id(cur, game)
                if game_id is None:
                    skipped_unmatched += 1
                    continue
                matched += 1

                payload = dict(game)
                payload["imported_at_utc"] = datetime.now(timezone.utc).isoformat()
                result_hash = compute_result_hash(game)
                captured_at = datetime.now(timezone.utc)
                status = str(game.get("status") or "unknown")
                home_score = parse_score(game.get("home_score"))
                away_score = parse_score(game.get("away_score"))

                cur.execute(
                    """
                    INSERT INTO game_results (
                        game_id,
                        source,
                        external_id,
                        status,
                        home_score,
                        away_score,
                        result_hash,
                        raw_json,
                        captured_at
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s)
                    ON CONFLICT (game_id, source, result_hash) DO NOTHING
                    """,
                    (
                        game_id,
                        RESULTS_SOURCE,
                        str(game.get("game_id") or ""),
                        status,
                        home_score,
                        away_score,
                        result_hash,
                        json.dumps(payload, separators=(",", ":"), ensure_ascii=False),
                        captured_at,
                    ),
                )
                if cur.rowcount > 0:
                    inserted += 1
        conn.commit()

    print(f"matched_games: {matched}")
    print(f"game_results_inserted: {inserted}")
    print(f"non_final_skipped: {skipped_nonfinal}")
    print(f"unmatched_skipped: {skipped_unmatched}")


def resolve_date_range(season: Optional[int], start_raw: Optional[str], end_raw: Optional[str]) -> tuple[date, date]:
    if start_raw and end_raw:
        return parse_date(start_raw), parse_date(end_raw)
    if season is not None:
        return date(season, 3, 1), date(season, 11, 30)
    today = datetime.now(timezone.utc).date()
    return today - timedelta(days=7), today


def parse_date(value: str) -> date:
    return datetime.strptime(value, "%Y-%m-%d").date()


def fetch_schedule(start_date: date, end_date: date) -> Iterable[Dict[str, Any]]:
    # MLB-StatsAPI wrapper over the public MLB stats API.
    return statsapi.schedule(
        start_date=start_date.isoformat(),
        end_date=end_date.isoformat(),
        sportId=1,
    )


def is_final_status(game: Dict[str, Any]) -> bool:
    status = str(game.get("status") or "").lower()
    return status.startswith("final")


def find_game_id(cur: psycopg.Cursor, game: Dict[str, Any]) -> Optional[int]:
    home_team = str(game.get("home_name") or "").strip()
    away_team = str(game.get("away_name") or "").strip()
    raw_game_date = str(game.get("game_date") or "")[:10]
    if not home_team or not away_team or not raw_game_date:
        return None
    try:
        game_date = parse_date(raw_game_date)
    except ValueError:
        return None
    game_dt = parse_game_datetime(game.get("game_datetime"), game_date)

    cur.execute(
        """
        SELECT id
        FROM games
        WHERE sport = 'MLB'
          AND home_team = %s
          AND away_team = %s
          AND (commence_time AT TIME ZONE 'UTC')::date = %s
        ORDER BY ABS(EXTRACT(EPOCH FROM (commence_time - %s)))
        LIMIT 1
        """,
        (home_team, away_team, game_date, game_dt),
    )
    row = cur.fetchone()
    if row is None:
        return None
    return int(row[0])


def parse_game_datetime(raw: Any, fallback_date: date) -> datetime:
    if raw is None:
        return datetime(fallback_date.year, fallback_date.month, fallback_date.day, tzinfo=timezone.utc)
    text = str(raw).strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return datetime(fallback_date.year, fallback_date.month, fallback_date.day, tzinfo=timezone.utc)
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def parse_score(value: Any) -> Optional[int]:
    if value is None:
        return None
    text = str(value).strip()
    if text == "":
        return None
    try:
        return int(text)
    except ValueError:
        return None


def compute_result_hash(game: Dict[str, Any]) -> str:
    payload = {
        "game_id": game.get("game_id"),
        "status": game.get("status"),
        "home_score": parse_score(game.get("home_score")),
        "away_score": parse_score(game.get("away_score")),
    }
    raw = json.dumps(payload, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


if __name__ == "__main__":
    main()
