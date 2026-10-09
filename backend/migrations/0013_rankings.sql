-- +goose Up

-- A manager's own ranked list of players for one league, made ahead of a
-- draft. It may be attached to the draft it is meant for; either way it can
-- be loaded into the manager's queue once a draft that covers the league
-- is under way. Nobody else sees it.
create table rankings (
  id           uuid primary key default gen_random_uuid(),
  franchise_id uuid not null references franchises(id) on delete cascade,
  league_id    uuid not null references leagues(id) on delete cascade,
  draft_id     uuid references drafts(id) on delete set null,
  name         text not null,
  updated_at   timestamptz not null default now()
);
create index rankings_franchise_idx on rankings (franchise_id);

create table ranking_players (
  ranking_id uuid not null references rankings(id) on delete cascade,
  player_id  uuid not null references players(id) on delete cascade,
  rank       int  not null,
  primary key (ranking_id, player_id)
);

-- +goose Down
drop table ranking_players;
drop table rankings;
