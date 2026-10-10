-- +goose Up
-- Separate from roster entries so dropping or trading cannot reset a season lock.
create table injury_reserve_locks (
 season_id uuid not null references seasons(id) on delete cascade,
 player_id uuid not null references players(id) on delete cascade,
 franchise_id uuid not null references franchises(id) on delete cascade,
 replacement_id uuid not null references players(id),
 created_at timestamptz not null default now(),
 primary key (season_id, player_id)
);

-- +goose Down
drop table injury_reserve_locks;
