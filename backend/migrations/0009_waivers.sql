-- +goose Up

-- A dropped player waits here, in leagues that use waivers, until clears_at.
-- Until then he can only be claimed; after it the best claim gets him, or
-- he becomes a free agent.
create table waivers (
  league_id uuid not null references leagues(id),
  player_id uuid not null references players(id) on delete cascade,
  clears_at timestamptz not null,
  primary key (league_id, player_id)
);
create index waivers_clears_idx on waivers (clears_at);

-- A franchise's request for a player on waivers. Waiver priority and FAAB
-- spending are not stored: both are read from the claims that were won.
create table waiver_claims (
  id             uuid primary key default gen_random_uuid(),
  league_id      uuid not null references leagues(id),
  franchise_id   uuid not null references franchises(id),
  player_id      uuid not null references players(id) on delete cascade,
  drop_player_id uuid references players(id) on delete set null, -- released to make room if the claim wins
  bid            int  not null default 0 check (bid >= 0),
  status         text not null default 'pending' check (status in ('pending', 'won', 'lost', 'cancelled')),
  reason         text not null default '', -- why a claim lost
  created_at     timestamptz not null default now(),
  resolved_at    timestamptz
);
create unique index waiver_claims_one_pending_idx on waiver_claims (league_id, franchise_id, player_id) where status = 'pending';
create index waiver_claims_franchise_idx on waiver_claims (franchise_id, league_id);

-- Leagues made before these rules existed get them switched off.
update leagues set settings = settings
  || jsonb_build_object('waivers', jsonb_build_object('mode', 'none', 'days', 2, 'budget', 100))
  || jsonb_build_object('free_agency', settings->'free_agency' || jsonb_build_object('weekly_limit', 0))
  || jsonb_build_object('roster', jsonb_build_object('reserve_lock_days', 0) || (settings->'roster'))
where not settings ? 'waivers';

-- +goose Down
update leagues set settings = (settings - 'waivers') #- '{free_agency,weekly_limit}';
drop table waiver_claims;
drop table waivers;
