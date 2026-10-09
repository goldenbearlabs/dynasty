-- +goose Up

-- A real game. day is the sports day it belongs to (US Eastern), which is
-- what lineups and scoring are keyed on.
create table games (
  id              uuid primary key default gen_random_uuid(),
  competition     text not null,
  provider_id     text not null,
  day             date not null,
  starts_at       timestamptz not null, -- lineup locks read this
  status          text not null check (status in ('scheduled', 'live', 'final')),
  home_team_id    uuid references pro_teams(id),
  away_team_id    uuid references pro_teams(id),
  stats_synced_at timestamptz,
  unique (competition, provider_id)
);
create index games_day_idx on games (competition, day);

-- What one player did in one game, as canonical stat keys: {"pts":30,"reb":3}.
-- Fantasy points are never stored; they are these numbers times the
-- league's scoring rules, worked out when asked for.
create table stat_lines (
  game_id   uuid not null references games(id) on delete cascade,
  player_id uuid not null references players(id) on delete cascade,
  stats     jsonb not null,
  primary key (game_id, player_id)
);
create index stat_lines_player_idx on stat_lines (player_id);

-- A league lives for years; each year is a season. Points count for games
-- between starts_on and ends_on.
create table seasons (
  id                    uuid primary key default gen_random_uuid(),
  league_id             uuid not null references leagues(id),
  year                  int  not null,
  starts_on             date not null,
  ends_on               date not null,
  status                text not null default 'active' check (status in ('active', 'complete')),
  champion_franchise_id uuid references franchises(id),
  unique (league_id, year),
  check (ends_on >= starts_on)
);
create unique index seasons_one_active_idx on seasons (league_id) where status = 'active';

-- A lineup takes effect on a day and stays in force until a later one. The
-- header row exists even when nobody starts, so an empty lineup is not
-- mistaken for "no change".
create table lineups (
  league_id    uuid not null references leagues(id),
  franchise_id uuid not null references franchises(id),
  effective_on date not null,
  primary key (league_id, franchise_id, effective_on)
);

create table lineup_entries (
  league_id    uuid not null,
  franchise_id uuid not null,
  effective_on date not null,
  slot         text not null, -- a slot name from the league's lineup rules
  slot_index   int  not null,
  player_id    uuid not null references players(id) on delete cascade,
  primary key (league_id, franchise_id, effective_on, slot, slot_index),
  foreign key (league_id, franchise_id, effective_on) references lineups on delete cascade
);
create index lineup_entries_player_idx on lineup_entries (player_id);

-- Weekly lineups need to know which day a week starts on.
update leagues
set settings = jsonb_set(settings, '{lineup,week_start}',
                         case when competition = 'nfl' then '"tuesday"' else '"monday"' end::jsonb)
where settings->'lineup' is not null and not (settings->'lineup' ? 'week_start');

-- +goose Down
drop table lineup_entries;
drop table lineups;
drop table seasons;
drop table stat_lines;
drop table games;
