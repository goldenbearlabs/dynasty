-- +goose Up
-- When each franchise set its roster for a season: who is on the main roster
-- and who on reserve. From then on, in a league that locks the reserve list
-- for the season, players stay on their lists until the season ends.
create table season_rosters (
  season_id    uuid not null references seasons(id) on delete cascade,
  franchise_id uuid not null references franchises(id) on delete cascade,
  set_at       timestamptz not null default now(),
  primary key (season_id, franchise_id)
);

-- +goose Down
drop table season_rosters;
