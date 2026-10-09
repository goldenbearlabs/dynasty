-- +goose Up

-- A draft covers one league, or several for a combined startup draft.
-- The clock is a stored deadline so it survives a restart.
create table drafts (
  id                 uuid primary key default gen_random_uuid(),
  dynasty_id         uuid not null references dynasties(id),
  name               text not null,
  kind               text not null check (kind in ('startup', 'seasonal')),
  year               int  not null,
  status             text not null default 'scheduled' check (status in ('scheduled', 'live', 'paused', 'complete')),
  pick_clock_seconds int  not null default 0, -- 0 means untimed
  clock_expires_at   timestamptz,
  created_at         timestamptz not null default now(),
  completed_at       timestamptz
);

create table draft_leagues (
  draft_id  uuid not null references drafts(id) on delete cascade,
  league_id uuid not null references leagues(id),
  primary key (draft_id, league_id)
);

-- A pick is a tradeable asset before it is used and a record after.
-- position is its place in the draft's overall order.
create table draft_picks (
  id                    uuid primary key default gen_random_uuid(),
  draft_id              uuid not null references drafts(id) on delete cascade,
  round                 int  not null,
  position              int  not null,
  original_franchise_id uuid not null references franchises(id),
  current_franchise_id  uuid not null references franchises(id),
  -- filled when the pick is made
  player_id             uuid references players(id),
  league_id             uuid references leagues(id),
  picked_at             timestamptz,
  auto_picked           boolean not null default false,
  -- set when the clock ran out; the franchise may still make the pick later
  skipped_at            timestamptz,
  -- deferred so the commissioner can reorder picks within one transaction
  unique (draft_id, position) deferrable initially deferred
);

-- Each manager's ranked wish list, used when their clock runs out.
create table draft_queue (
  draft_id     uuid not null references drafts(id) on delete cascade,
  franchise_id uuid not null references franchises(id),
  player_id    uuid not null references players(id) on delete cascade,
  rank         int  not null,
  primary key (draft_id, franchise_id, player_id)
);

alter table transactions add column draft_pick_id uuid references draft_picks(id) on delete set null;

-- +goose Down
alter table transactions drop column draft_pick_id;
drop table draft_queue;
drop table draft_picks;
drop table draft_leagues;
drop table drafts;
