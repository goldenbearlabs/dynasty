-- +goose Up

-- What a player did in each season, kept so any page can show it without
-- asking a feed. competition is where the season was played, which is not
-- always where the player is now: an NBA player keeps his college seasons.
-- league is empty for the competition's own league and names any other,
-- such as a junior league. Fantasy points are not stored; they are these
-- totals times a league's scoring rules.
create table player_seasons (
  player_id   uuid not null references players(id) on delete cascade,
  competition text not null,
  year        int  not null, -- the feed's number for the season, used for ordering
  label       text not null, -- "2025-26", "2025"
  team        text not null default '',
  league      text not null default '',
  games       int  not null default 0,
  stats       jsonb not null,
  synced_at   timestamptz not null default now(),
  primary key (player_id, competition, year, team, league)
);
create index player_seasons_season_idx on player_seasons (competition, year);

-- When a player's seasons outside his own league (juniors, international)
-- were last looked up; those come one player at a time.
alter table players add column career_checked_at timestamptz;

-- +goose Down
alter table players drop column career_checked_at;
drop table player_seasons;
