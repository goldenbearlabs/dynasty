#!/usr/bin/env python3
"""Publish CBB model snapshots immediately and periodically, without retraining.

Bulk archives seed a persistent, separate live corpus. ESPN completed-game
summaries correct recent history; current rosters enumerate prospective targets.
Only closed prior sports days enter features. Failed runs retain live forecasts.
"""
import argparse
from collections import defaultdict
import datetime as dt
import fcntl
import gzip
import hashlib
import json
import logging
import re
from pathlib import Path
import shutil
import subprocess
import sys
import time
import urllib.request

import numpy as np
import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.parquet as pq
from cbb_dataset import D1_CONFERENCES, ELIGIBLE, STATS, sports_day, validate_stats
from cbb_features import Dataset, State, NAMES
from cbb_context_features import ContextState, Recruiting, NAMES as CONTEXT_NAMES
from cbb_context_sources import normalized
from cbb_context_model import predict
from cbb_predict import validate_target
from cbb_strength import day_number
from nba_history import parse_boxscore

BASE = 'https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball'
LOG = logging.getLogger('cbb-refresh')
UTC = dt.timezone.utc


def timestamp(value):
    result = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
    if result.tzinfo is None:
        raise ValueError('Timezone required')
    return result


def atomic_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_suffix('.tmp')
    temp.write_text(json.dumps(value, separators=(',', ':'), allow_nan=False) + '\n')
    temp.replace(path)


class Feed:
    def __init__(self, folder):
        self.folder = folder
        self.captures = []

    def fetch(self, name, url):
        for attempt in range(3):
            try:
                request = urllib.request.Request(url, headers={'User-Agent': 'mosaic-cbb-model/1.0'})
                with urllib.request.urlopen(request, timeout=45) as response:
                    body = response.read()
                value = json.loads(body)
                (self.folder / (name + '.json')).write_bytes(body)
                self.captures.append({'url': url, 'observed_at': dt.datetime.now(UTC).isoformat(),
                                      'sha256': hashlib.sha256(body).hexdigest()})
                time.sleep(.15)
                return value
            except Exception:
                if attempt == 2:
                    raise
                time.sleep(2 ** attempt)


def seed(root, today):
    """Cold deployments bootstrap themselves; research caches are not required."""
    if not (root / 'history.parquet').exists():
        ending = today.year + (today.month >= 7)
        seasons = ','.join(map(str, range(2023, ending + 1)))
        LOG.info('First-run history download: seasons %s', seasons)
        subprocess.run([sys.executable, 'tools/cbb_history.py', '--seasons', seasons,
                        '--out', str(root / 'raw')], check=True)
        (root / 'corpus-writing').touch()
        subprocess.run([sys.executable, 'tools/cbb_dataset.py', '--seasons', seasons,
                        '--raw', str(root / 'raw'), '--out', str(root),
                        '--through', (today - dt.timedelta(days=1)).isoformat()], check=True)
        (root / 'corpus-writing').unlink()
    if not (root / 'context/recruiting.json').exists():
        subprocess.run([sys.executable, 'tools/cbb_context_sources.py', '--recruits-only',
                        '--out', str(root / 'context')], check=True)


def summary_rows(payload, conferences):
    """Normalize an entire completed game, with the research integrity checks."""
    header = payload.get('header', {})
    kind = int(header.get('season', {}).get('type', 0))
    if kind not in (2, 3):
        raise ValueError('Unsupported season type')
    # Shared NBA parser only admits type 2. CBB postseason has the same schema.
    parsed = parse_boxscore({**payload, 'header': {**header, 'season': {**header['season'], 'type': 2}}})
    c = header['competitions'][0]
    sides = c['competitors']
    home = int(next(s['team']['id'] for s in sides if s['homeAway'] == 'home'))
    away = int(next(s['team']['id'] for s in sides if s['homeAway'] == 'away'))
    starts = timestamp(c['date'])
    day = sports_day(starts)
    overtime = re.search(r'/([0-9]*)OT', c['status']['type'].get('detail', ''))
    periods = int(c['status'].get('period', 2 + (int(overtime[1] or 1) if overtime else 0)))
    duration = 40 + 5 * max(0, periods - 2)
    scores = {int(s['team']['id']): float(s['score']) for s in sides}
    if any(abs(sum(r['stats'].get('pts', 0) for r in parsed if int(r['team']) == t) - score) > .01
           for t, score in scores.items()):
        raise ValueError('Player/team score mismatch')
    possessions = []
    for side in payload['boxscore']['teams']:
        stats = {v['name']: v.get('displayValue') for v in side['statistics']}
        fga = float(stats['fieldGoalsMade-fieldGoalsAttempted'].split('-')[1])
        fta = float(stats['freeThrowsMade-freeThrowsAttempted'].split('-')[1])
        possessions.append(fga + .475 * fta - float(stats['offensiveRebounds']) + float(stats['totalTurnovers'] if 'totalTurnovers' in stats else stats['turnovers']))
    pace_poss = float(np.mean(possessions))
    if len(possessions) != 2 or not 25 < pace_poss < 180:
        raise ValueError('Invalid possessions')
    rows = []
    for r in parsed:
        team, opponent = int(r['team']), int(r['opponent'])
        played = r['played'] and r['minutes'] > 0 and not r['minutes_missing']
        stats = {k: float(r['stats'].get(k, 0)) for k in STATS + ['tpa', 'oreb', 'dreb', 'pf']}
        if played and (any(k not in r['stats'] for k in STATS + ['tpa', 'pf']) or
                       not validate_stats(stats) or not 0 < r['minutes'] <= duration + 1):
            raise ValueError('Invalid playing stat line')
        rows.append({'game_id': int(r['game_id']), 'player_id': int(r['player_id']), 'name': r['name'],
                     'team': team, 'opponent': opponent, 'season': r['season'], 'starts_at': starts.isoformat(),
                     'day': day, 'position': r['position'] or 'UNK', 'minutes': float(r['minutes']) if played else 0.,
                     'played': bool(played), 'starter': bool(r['starter']), 'home': team == home,
                     'neutral': bool(c.get('neutralSite')), 'neutral_known': 'neutralSite' in c,
                     'conference': conferences.get(team, 0), 'opponent_conference': conferences.get(opponent, 0),
                     'd1_matchup': all(conferences.get(t, 0) in D1_CONFERENCES for t in [home, away]),
                     'eligible': conferences.get(team, 0) in ELIGIBLE, 'season_type': kind, **stats})
    if len({r['player_id'] for r in rows}) != len(rows):
        raise ValueError('Duplicate player identity')
    game = {'game_id': int(header['id']), 'season': int(header['season']['year']), 'starts_at': starts.isoformat(),
            'day': day, 'home': home, 'away': away, 'neutral': bool(c.get('neutralSite')),
            'neutral_known': 'neutralSite' in c, 'conference_game': bool(c.get('conferenceCompetition')),
            'season_type': kind, 'duration': duration, 'possessions': pace_poss, 'pace': pace_poss * 40 / duration,
            'teams': [{'team': t, 'opponent': away if t == home else home, 'score': scores[t],
                       'conference': conferences.get(t, 0)} for t in [home, away]],
            'd1_matchup': all(conferences.get(t, 0) in D1_CONFERENCES for t in [home, away])}
    return rows, game


def merge_corpus(root, rows, games):
    if not games:
        return
    replaced = {g['game_id'] for g in games}
    table = pq.read_table(root / 'history.parquet')
    keep = pc.invert(pc.is_in(table['game_id'], value_set=pa.array(sorted(replaced), type=table['game_id'].type)))
    added = pa.Table.from_pylist(rows, schema=table.schema)
    merged = pa.concat_tables([table.filter(keep), added])
    merged = merged.take(pc.sort_indices(merged, sort_keys=[('starts_at', 'ascending'), ('game_id', 'ascending'), ('player_id', 'ascending')]))
    old_games = json.loads((root / 'games.json').read_text())
    all_games = sorted([g for g in old_games if g['game_id'] not in replaced] + games, key=lambda g: (g['starts_at'], g['game_id']))
    # The private corpus is rebuilt from source if interrupted between these two
    # replacements. It is never read by the app or published as a forecast.
    marker = root / 'corpus-writing'
    marker.touch()
    pq.write_table(merged, root / 'history.tmp', compression='zstd')
    (root / 'history.tmp').replace(root / 'history.parquet')
    atomic_json(root / 'games.json', all_games)
    marker.unlink()


def replay(data, cutoff, recruits):
    """Replay causal state without the large, research-only feature matrices."""
    state = State(data)
    context = ContextState(data, recruits)
    a = data.a
    days, starts = np.unique(a['day'], return_index=True)
    ends = np.r_[starts[1:], data.n]
    for day, first, last in zip(days, starts, ends):
        if day >= cutoff:
            break
        games = data.byday[int(day)]
        teams = {side['team'] for g in games for side in g['teams']}
        season = int(a['season'][first])
        state.ratings = state.strength.fit(season, int(day), teams)
        state.league = state.strength.league
        game_context = {g['game_id']: state.strength.context(g, state.ratings, state.league) for g in games}
        bases = {}
        for i in range(int(first), int(last)):
            if not a['played'][i] or a['minutes'][i] <= 0:
                continue
            h = [j for j in state.player_history(a['player_id'][i]) if a['season'][j] >= season - 2]
            position = int(data.positions[h[-1]]) if h else 0
            prior = state.priors(season, position, False)[1]
            if h:
                ix = np.asarray(h)
                weights = np.exp2(-np.arange(len(h)-1, -1, -1)/10) * np.power(.35, season-a['season'][ix])
                prior = (120*prior + weights @ data.stats[ix]) / (120 + float(weights @ a['minutes'][ix]))
            bases[i] = prior.astype(np.float32)
            c = game_context[int(a['game_id'][i])][int(a['team'][i])]
            state.prior_opp_def[i] = c['opp_def']
            state.prior_own_off[i] = c['own_off']
        state.observe(range(int(first), int(last)), games, bases)
        context.observe(range(int(first), int(last)))
    return state, context


def make_targets(events, rosters, conferences, observed):
    targets = []
    for event in events:
        c = event['competitions'][0]
        start = timestamp(c['date'])
        if start <= observed or c.get('status', event.get('status', {})).get('type', {}).get('state') != 'pre':
            continue
        sides = c['competitors']
        if len(sides) != 2 or 'neutralSite' not in c:
            raise ValueError('Incomplete scheduled matchup')
        for side in sides:
            team = int(side['team']['id'])
            opponent = next(int(s['team']['id']) for s in sides if s is not side)
            if team not in rosters:
                continue
            for player in rosters[team]:
                targets.append({'game_id': int(event['id']), 'player_id': int(player['id']),
                                'team': team, 'opponent': opponent, 'season': int(event['season']['year']),
                                'season_type': int(event['season']['type']), 'starts_at': start.isoformat(),
                                'home': side['homeAway'] == 'home', 'neutral': bool(c['neutralSite']),
                                'conference': conferences.get(team, 0), 'opponent_conference': conferences.get(opponent, 0)})
    return targets


def locked_records(old, now):
    """Started games retain their last pre-tip expectation across restarts."""
    return {k: r for k, r in old.get('records', {}).items()
            if now - dt.timedelta(days=30) < timestamp(r['target']['starts_at']) <= now
            and timestamp(r['forecast_at']) < timestamp(r['target']['starts_at'])}


def forecast(model, data, state, context, targets, observed):
    rows = []
    x, base, scale, shoot, ctx = [], [], [], [], []
    cutoff = day_number(sports_day(observed))
    for target in targets:
        validate_target(target, observed, model)
        team, opp, season = (int(target[k]) for k in ['team', 'opponent', 'season'])
        game = {'game_id': target['game_id'], 'season': season, 'season_type': target['season_type'],
                'home': team if target['home'] else opp, 'away': opp if target['home'] else team,
                'neutral': target['neutral'], 'teams': [{'team': t, 'opponent': o, 'conference': target[k]}
                for t, o, k in [(team, opp, 'conference'), (opp, team, 'opponent_conference')]]}
        if not rows or rows[-1]['season'] != season:
            teams = {int(t[k]) for t in targets if int(t['season']) == season for k in ['team', 'opponent']}
            state.ratings = state.strength.fit(season, cutoff, teams)
            state.league = state.strength.league
        state.context_cache = {}
        xx, b, s, h, meta = state.build(None, cutoff, game, target=target)
        x.append(xx); base.append(b); scale.append(s); shoot.append(h)
        ctx.append(context.build(None, cutoff, target)); rows.append(target)
    if not rows:
        return {}
    inputs = {'x': np.array(x), 'base': np.array(base), 'scales': np.array(scale), 'shoot': np.array(shoot)}
    minutes, stats, _ = predict(model, inputs, {'x': np.array(ctx)}, np.arange(len(rows)))
    records = {}
    for i, target in enumerate(rows):
        # A game can start while a long refresh is computing. Never issue a
        # post-tip prediction with a retroactively assigned pre-tip timestamp.
        issued = dt.datetime.now(UTC)
        if timestamp(target['starts_at']) <= issued:
            continue
        statline = dict(zip(STATS, map(float, stats[i])))
        if not np.isfinite(stats[i]).all() or min(statline.values()) < 0 or not .5 <= minutes[i] <= 40:
            raise ValueError('Invalid forecast')
        public_target = {k: str(target[k]) if k in ['game_id', 'player_id', 'team', 'opponent'] else target[k]
                         for k in ['game_id', 'player_id', 'team', 'opponent', 'starts_at', 'season', 'home']}
        key = str(target['game_id']) + '/' + str(target['player_id'])
        records[key] = {'target': public_target, 'prediction': {'minutes': float(minutes[i]), 'stats': statline},
                        'forecast_at': issued.isoformat(), 'out': False}
    return records


def refresh(args):
    root = Path(args.data)
    root.mkdir(parents=True, exist_ok=True)
    started = dt.datetime.now(UTC)
    day = dt.date.fromisoformat(sports_day(started))
    # A crash between private corpus replacements must not leave mixed versions.
    if (root / 'corpus-writing').exists():
        (root / 'history.parquet').unlink(missing_ok=True)
        (root / 'corpus-writing').unlink()
    seed(root, day)
    model_bytes = Path(args.model).read_bytes()
    model = json.loads(model_bytes)
    if model.get('sport') != 'cbb' or model.get('feature_names') != NAMES or not model.get('context_correction'):
        raise ValueError('Expected the approved CBB context model')
    spec = model['context_correction']
    if spec['feature_names'] != [NAMES[i] for i in spec['core_columns']] + CONTEXT_NAMES:
        raise ValueError('Context schema mismatch')
    folder = root / 'live-snapshots' / started.strftime('%Y%m%dT%H%M%S%fZ')
    folder.mkdir(parents=True)
    feed = Feed(folder)
    conferences = {}
    ending = day.year + (day.month >= 7)
    core = 'https://sports.core.api.espn.com/v2/sports/basketball/leagues/mens-college-basketball'
    for conf in sorted(D1_CONFERENCES):
        payload = feed.fetch('conference-' + str(conf), core + '/seasons/' + str(ending) + '/types/2/groups/' + str(conf) + '/teams?limit=500')
        if payload.get('pageCount', 1) > 1:
            raise ValueError('Conference membership truncated')
        for item in payload.get('items', []):
            team = int(re.search(r'/teams/(\d+)', item['$ref'])[1])
            if team in conferences and conferences[team] != conf:
                raise ValueError('Conflicting conference membership')
            conferences[team] = conf
    if len(conferences) < 300:
        raise ValueError('Incomplete Division I membership feed')
    old_games = json.loads((root / 'games.json').read_text())
    latest = max(g['day'] for g in old_games)
    first = min(day - dt.timedelta(days=3), dt.date.fromisoformat(latest))
    # No need to scan the off-season when starting the next season.
    if first < dt.date(day.year, 10, 1) and day.month >= 10:
        first = dt.date(day.year, 10, 1)
    events = {}
    for offset in range((day + dt.timedelta(days=args.horizon_days) - first).days + 1):
        date = first + dt.timedelta(days=offset)
        payload = feed.fetch('schedule-' + date.isoformat(), BASE + '/scoreboard?dates=' + date.strftime('%Y%m%d') + '&groups=50&limit=500')
        if len(payload.get('events', [])) >= 500:
            raise ValueError('Scoreboard may be truncated')
        for event in payload.get('events', []):
            if event.get('season', {}).get('type') in [2, 3]:
                events[str(event['id'])] = event
    new_rows, new_games, scheduled, excluded = [], [], [], []
    for game_id, event in events.items():
        status = event['status']['type']
        if status.get('completed') and sports_day(timestamp(event['date'])) < day.isoformat():
            payload = feed.fetch('game-' + game_id, BASE + '/summary?event=' + game_id)
            try:
                rows, game = summary_rows(payload, conferences)
            except (ValueError, KeyError) as error:
                excluded.append({'game_id': game_id, 'reason': str(error)})
                LOG.warning('Excluding corrupt/incomplete game %s: %s', game_id, error)
                continue
            new_rows.extend(rows); new_games.append(game)
        elif status.get('state') == 'pre':
            scheduled.append(event)
    if excluded and len(excluded) / (len(excluded) + len(new_games)) > .1:
        raise ValueError('Over ten percent of completed summaries failed integrity checks')
    merge_corpus(root, new_rows, new_games)
    data = Dataset(root)
    latest = max(g['day'] for g in data.games)
    # Detect a stale history feed during the season; the off-season has no
    # legitimate completed games and must still allow early preseason forecasts.
    age = (day - dt.date.fromisoformat(latest)).days
    if day.month in [11, 12, 1, 2, 3] and age > 7 and not (day.month == 11 and day.day <= 7):
        raise ValueError('CBB completed history is over seven sports days old')
    LOG.info('History ready: %s games, %s rows, through %s', len(data.games), data.n, latest)
    rosters = {}
    needed = {int(side['team']['id']) for e in scheduled for side in e['competitions'][0]['competitors']}
    for team in sorted(needed & set(conferences)):
        payload = feed.fetch('roster-' + str(team), BASE + '/teams/' + str(team) + '/roster')
        people = payload.get('athletes', [])
        if not people or len({p['id'] for p in people}) != len(people):
            raise ValueError('Empty/duplicate roster ' + str(team))
        rosters[team] = people
    names = defaultdict(set)
    for player, name in zip(data.a['player_id'], data.table['name'].to_pylist()):
        names[int(player)].add(normalized(name))
    for people in rosters.values():
        for p in people:
            names[int(p['id'])].add(normalized(p.get('displayName', p.get('fullName', ''))))
    recruits = Recruiting(root / 'context/recruiting.json', names)
    observed = dt.datetime.now(UTC)
    if sports_day(observed) != day.isoformat():
        raise ValueError('Sports day changed during collection; retry on the next run')
    targets = make_targets(scheduled, rosters, conferences, observed)
    LOG.info('Replaying historical state for %s targets', len(targets))
    state, context = replay(data, day_number(day.isoformat()), recruits)
    records = forecast(model, data, state, context, targets, observed)
    live = root / 'live-projections.json'
    old = json.loads(live.read_text()) if live.exists() else {}
    completed = dt.datetime.now(UTC)
    preserved = locked_records(old, completed)
    preserved.update(records)
    through = max(timestamp(g['starts_at']) for g in data.games if g['day'] < day.isoformat())
    snapshot = {'version': 1, 'sport': 'cbb', 'generated_at': completed.isoformat(),
                'history_through': through.isoformat(), 'model_sha256': hashlib.sha256(model_bytes).hexdigest(),
                'records': preserved}
    atomic_json(folder / 'manifest.json', {'started_at': started.isoformat(), 'information_cutoff': observed.isoformat(),
                'captures': feed.captures, 'excluded_games': excluded, 'new_forecasts': len(records), 'preserved_started_forecasts': len(preserved)-len(records),
                'note': 'Conditional on playing; no calibrated DNP probabilities or timestamped injury model. No retraining.'})
    archive = root / 'forecast-archive'
    archive.mkdir(exist_ok=True)
    with gzip.open(archive / (started.strftime('%Y%m%dT%H%M%SZ') + '.json.gz'), 'wt') as out:
        json.dump(snapshot, out, allow_nan=False)
    atomic_json(live, snapshot)
    atomic_json(root / 'last-refresh.json', {'completed_at': completed.isoformat(), 'duration_seconds': (completed-started).total_seconds(),
                'new_forecasts': len(records), 'locked_forecasts': len(preserved)-len(records), 'history_through': latest,
                'model_sha256': snapshot['model_sha256'], 'capture': str(folder)})
    for child in (root / 'live-snapshots').iterdir():
        if child.is_dir() and child.stat().st_mtime < time.time() - 7*86400:
            shutil.rmtree(child)
    for child in archive.glob('*.json.gz'):
        if child.stat().st_mtime < time.time() - 90*86400:
            child.unlink()
    LOG.info('Published %s CBB forecasts in %.1fs', len(records), (completed-started).total_seconds())


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--data', default='data/cbb/live')
    p.add_argument('--model', default='models/cbb/model.json')
    p.add_argument('--horizon-days', type=int, default=7)
    p.add_argument('--interval-seconds', type=int, default=7200)
    p.add_argument('--loop', action='store_true', help='Run immediately, then periodically; retry failures after five minutes')
    args = p.parse_args()
    if not 1 <= args.horizon_days <= 14 or args.interval_seconds < 60:
        p.error('Invalid horizon or refresh interval')
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s', stream=sys.stdout)
    root = Path(args.data)
    root.mkdir(parents=True, exist_ok=True)
    with (root / 'refresh.lock').open('w') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            LOG.info('Another CBB worker owns the refresh lock')
            return
        while True:
            delay = args.interval_seconds
            try:
                refresh(args)
            except Exception as error:
                LOG.exception('CBB refresh failed; retaining previous forecasts: %s', error)
                if not args.loop:
                    raise
                delay = min(300, args.interval_seconds)
            if not args.loop:
                break
            LOG.info('Next CBB refresh in %s seconds', delay)
            time.sleep(delay)


if __name__ == '__main__':
    main()
