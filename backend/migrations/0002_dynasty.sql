-- +goose Up

create table dynasties (
  id         uuid primary key default gen_random_uuid(),
  name       text not null,
  settings   jsonb not null,
  created_at timestamptz not null default now()
);

-- One franchise per manager, spanning every league in the dynasty.
-- invite_token is the manager's sign-in credential.
create table franchises (
  id              uuid primary key default gen_random_uuid(),
  dynasty_id      uuid not null references dynasties(id),
  name            text not null,
  manager_name    text not null,
  slug            text not null,
  invite_token    text not null unique,
  is_commissioner boolean not null default false,
  unique (dynasty_id, slug)
);

-- One league per competition. last_draft_at is set when a draft completes;
-- players who entered the pool after it wait for the next draft.
create table leagues (
  id            uuid primary key default gen_random_uuid(),
  dynasty_id    uuid not null references dynasties(id),
  competition   text not null,
  name          text not null,
  settings      jsonb not null,
  last_draft_at timestamptz,
  unique (dynasty_id, competition)
);

create table roster_entries (
  league_id    uuid not null references leagues(id),
  franchise_id uuid not null references franchises(id),
  player_id    uuid not null references players(id),
  list         text not null check (list in ('main', 'reserve')),
  acquired_via text not null,
  acquired_at  timestamptz not null default now(),
  primary key (league_id, player_id) -- a player is on one roster per league
);
create index roster_entries_franchise_idx on roster_entries (franchise_id);

-- Append-only: the activity feed and the audit trail.
create table transactions (
  id           bigserial primary key,
  dynasty_id   uuid not null references dynasties(id),
  league_id    uuid references leagues(id),
  franchise_id uuid references franchises(id),
  kind         text not null,
  player_id    uuid references players(id),
  detail       jsonb not null default '{}',
  created_at   timestamptz not null default now()
);

-- +goose Down
drop table transactions;
drop table roster_entries;
drop table leagues;
drop table franchises;
drop table dynasties;
