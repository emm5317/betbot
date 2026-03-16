#!/usr/bin/env python
"""Wrapper for importing normalized free-scraper CSV odds files.

This script is intended for outputs produced from:
  - github.com/ArnavSaraogi/mlb-odds-scraper
  - github.com/flancast90/sportsbookreview-scraper

Normalize those outputs to the CSV schema accepted by `import_historical_odds.py`
(`date,start_time,away,home,away_ml,home_ml,total,over,under,home_rl_spread,rl_away,rl_home,book_key,book_name,source,...`)
and pass them here.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Import normalized scraped MLB odds CSV files")
    parser.add_argument("--input", action="append", required=True, help="Path to normalized scraped CSV (repeatable)")
    parser.add_argument(
        "--database-url",
        default=os.getenv("BETBOT_DATABASE_URL", ""),
        help="Postgres connection string (defaults to BETBOT_DATABASE_URL)",
    )
    parser.add_argument("--dry-run", action="store_true", help="Parse and validate only")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    script = Path(__file__).with_name("import_historical_odds.py")
    cmd = [sys.executable, str(script), "--skip-xlsx"]
    for path in args.input:
        cmd.extend(["--scraped-csv", path])
    if args.database_url:
        cmd.extend(["--database-url", args.database_url])
    if args.dry_run:
        cmd.append("--dry-run")

    completed = subprocess.run(cmd, check=False)
    raise SystemExit(completed.returncode)


if __name__ == "__main__":
    main()
