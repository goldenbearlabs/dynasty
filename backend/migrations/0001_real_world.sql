-- +goose Up

create table pro_teams (
  id          uuid primary key default gen_random_uuid(),
  competition text not null,
  provider_id text not null,
  abbrev      text not null,
  name        text not null,
  logo_url    text not null default '',
  unique (competition, provider_id)
);

-- One person is one row for their whole career. competition changes when
-- they move (cbb -> nba); eligible_since records when they entered that pool.
create table players (
  id             uuid primary key default gen_random_uuid(),
  competition    text not null,
  status         text not null check (status in ('prospect', 'active', 'inactive')),
  eligible_since timestamptz not null default now(),
  full_name      text not null,
  positions      text[] not null default '{}',
  birth_date     date,
  pro_team_id    uuid references pro_teams(id),
  class          text not null default '',
  note           text not null default '',
  headshot_url   text not null default '',
  updated_at     timestamptz not null default now()
);
create index players_competition_idx on players (competition, status);

-- Fetchers find players through this table, so a re-run never duplicates anyone.
create table player_external_ids (
  provider    text not null,
  provider_id text not null,
  player_id   uuid not null references players(id) on delete cascade,
  primary key (provider, provider_id)
);

create table ingest_runs (
  id            bigserial primary key,
  competition   text not null,
  job           text not null,
  started_at    timestamptz not null default now(),
  finished_at   timestamptz,
  status        text not null default 'running' check (status in ('running', 'ok', 'error')),
  rows_upserted int not null default 0,
  error         text not null default ''
);

-- The latest response body per URL, kept so a feed change can be diagnosed.
create table raw_payloads (
  url        text primary key,
  body       text not null,
  fetched_at timestamptz not null default now()
);

-- +goose Down
drop table raw_payloads;
drop table ingest_runs;
drop table player_external_ids;
drop table players;
drop table pro_teams;
