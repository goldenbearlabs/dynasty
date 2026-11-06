#!/usr/bin/env python3
"""Download regular-season ESPN NBA box scores, with a resumable raw cache.

Standard library only. Does not write to the application database. Published
box-score positions/starts are retained as outcomes, never pregame features.
"""
import argparse
import concurrent.futures
import datetime as dt
import json
from pathlib import Path
import threading
import time
import urllib.request

BASE = "https://site.api.espn.com/apis/site/v2/sports/basketball/nba"
LOCK = threading.Lock()
NEXT = 0.0
KEYS = {"minutes": "min", "points": "pts", "rebounds": "reb", "assists": "ast",
        "steals": "stl", "blocks": "blk", "turnovers": "tov",
        "fieldGoalsMade": "fgm", "fieldGoalsAttempted": "fga",
        "threePointFieldGoalsMade": "tpm", "threePointFieldGoalsAttempted": "tpa",
        "freeThrowsMade": "ftm", "freeThrowsAttempted": "fta",
        "offensiveRebounds": "oreb", "defensiveRebounds": "dreb", "fouls": "pf"}


def fetch(url, path, gap):
    global NEXT
    if path.exists():
        return json.loads(path.read_text())
    for attempt in range(3):
        with LOCK:
            delay = max(0, NEXT - time.monotonic())
            NEXT = max(NEXT, time.monotonic()) + gap
        time.sleep(delay)
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "crossover-dynasty/0.1"})
            with urllib.request.urlopen(request, timeout=40) as response:
                body = response.read()
            data = json.loads(body)
            path.parent.mkdir(parents=True, exist_ok=True)
            temporary = path.with_suffix(".tmp")
            temporary.write_bytes(body)
            temporary.replace(path)
            return data
        except Exception:
            if attempt == 2:
                raise
            time.sleep(2 ** (attempt + 1))


def parse_boxscore(data):
    header = data.get("header", {})
    if header.get("season", {}).get("type") != 2:
        return []
    competitions = header.get("competitions", [])
    if not competitions or not competitions[0].get("status", {}).get("type", {}).get("completed"):
        return []
    competition = competitions[0]
    sides = competition.get("competitors", [])
    teams = {str(side["team"]["id"]): side for side in sides}
    if len(teams) != 2:
        raise ValueError("expected two teams")
    rows = []
    for team in data.get("boxscore", {}).get("players", []):
        team_id = str(team["team"]["id"])
        opponent = next(key for key in teams if key != team_id)
        for group in team.get("statistics", []):
            for athlete in group.get("athletes", []):
                person = athlete["athlete"]
                stats = {}
                for key, value in zip(group.get("keys", []), athlete.get("stats", [])):
                    names = key.split("-")
                    values = value.split("-") if len(names) > 1 else [value]
                    for name, number in zip(names, values):
                        if name not in KEYS:
                            continue
                        try:
                            if name == "minutes" and ":" in number:
                                minutes, seconds = number.split(":")
                                stats["min"] = float(minutes) + float(seconds) / 60
                            else:
                                stats[KEYS[name]] = float(number)
                        except ValueError:
                            pass
                dnp = athlete.get("didNotPlay", False)
                if not person.get("id"):
                    if dnp or not any(stats.values()):
                        # Some historical DNP/zero-exposure entries contain
                        # only a surname and cannot be matched to a player.
                        continue
                    raise ValueError(f"appearance missing player ID: {header['id']}")
                missing_minutes = not dnp and "min" not in stats
                # Keep unknown minutes explicit. The model excludes these
                # rows instead of interpreting an absent value as exposure.
                tens = sum(stats.get(key, 0) >= 10 for key in ("pts", "reb", "ast", "stl", "blk"))
                stats.update(dd=float(tens >= 2), td=float(tens >= 3),
                             pts40=float(stats.get("pts", 0) >= 40), pts50=float(stats.get("pts", 0) >= 50))
                timestamp = dt.datetime.fromisoformat(competition["date"].replace("Z", "+00:00")).isoformat(timespec="seconds")
                rows.append(dict(game_id=str(header["id"]), starts_at=timestamp,
                                 season=int(header["season"]["year"]), player_id=str(person["id"]),
                                 name=person.get("displayName", ""), team=team_id, opponent=opponent,
                                 position=person.get("position", {}).get("abbreviation", ""),
                                 home=teams[team_id].get("homeAway") == "home",
                                 minutes=stats.pop("min", 0), played=not dnp and bool(athlete.get("stats")),
                                 minutes_missing=missing_minutes,
                                 starter=athlete.get("starter", False), stats=stats))
    if not rows or {row["team"] for row in rows} != set(teams):
        raise ValueError(f"empty completed box score: {header.get('id')}")
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--from", dest="since", required=True)
    parser.add_argument("--to", dest="until", required=True)
    parser.add_argument("--cache", default="data/nba/raw")
    parser.add_argument("--out", default="data/nba/history.jsonl")
    parser.add_argument("--gap", type=float, default=0.25)
    parser.add_argument("--workers", type=int, default=4)
    args = parser.parse_args()
    since, until = dt.date.fromisoformat(args.since), dt.date.fromisoformat(args.until)
    if since > until or args.gap < 0.1 or not 1 <= args.workers <= 8:
        parser.error("invalid date range, gap (minimum 0.1s), or worker count (1..8)")
    cache = Path(args.cache)
    games = {}
    days = [since + dt.timedelta(days=i) for i in range((until - since).days + 1)]

    def schedule(day):
        dates = day.strftime("%Y%m%d")
        return fetch(f"{BASE}/scoreboard?dates={dates}", cache / f"schedule-{dates}.json", args.gap)

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
        for index, payload in enumerate(executor.map(schedule, days), 1):
            events = payload.get("events", [])
            if len(events) >= 100:
                raise ValueError("scoreboard may be truncated")
            for event in events:
                kind = event.get("competitions", [{}])[0].get("type", {}).get("abbreviation")
                # ESPN labels All-Star and the Cup final as season type 2 as
                # well. Neither belongs in regular-season training outcomes.
                if kind == "STD" and event.get("season", {}).get("type") == 2 and event.get("status", {}).get("type", {}).get("completed"):
                    games[str(event["id"])] = event["date"]
            if index % 100 == 0:
                print(f"Schedules {index}/{len(days)} days; {len(games)} games", flush=True)
    print(f"Found {len(games)} completed regular-season games", flush=True)
    if not games:
        raise ValueError("no completed games in requested range")
    output = Path(args.out)
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(".tmp")

    def download(game):
        payload = fetch(f"{BASE}/summary?event={game}", cache / f"game-{game}.json", args.gap)
        rows = parse_boxscore(payload)
        if not rows or any(row["game_id"] != game for row in rows):
            raise ValueError(f"incomplete or mismatched box score: {game}")
        return rows

    # Preserve chronological output even when downloads finish out of order.
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor, temporary.open("w") as target:
        for index, rows in enumerate(executor.map(download, sorted(games, key=lambda g: (games[g], g))), 1):
            for row in rows:
                target.write(json.dumps(row, separators=(",", ":")) + "\n")
            if index % 100 == 0 or index == len(games):
                target.flush()
                print(f"{index}/{len(games)} games", flush=True)
    temporary.replace(output)
    print(f"Wrote {output}", flush=True)


if __name__ == "__main__":
    main()
