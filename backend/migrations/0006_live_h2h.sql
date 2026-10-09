-- +goose Up

-- The score and state of a game as it is played. detail is the feed's own
-- short description: "7:32 - 3rd", "Top 5th", "Final/OT".
alter table games
  add column home_score int  not null default 0,
  add column away_score int  not null default 0,
  add column detail     text not null default '';

-- Head-to-head leagues divide a season into periods. Each period pairs the
-- franchises off; the last few periods are the playoffs.
create table periods (
  id         uuid primary key default gen_random_uuid(),
  season_id  uuid not null references seasons(id) on delete cascade,
  seq        int  not null,
  starts_on  date not null,
  ends_on    date not null,
  is_playoff boolean not null default false,
  unique (season_id, seq)
);

-- Results are never stored: a matchup's score is each side's points over
-- its period. away_franchise_id is null for a bye.
create table matchups (
  id                uuid primary key default gen_random_uuid(),
  period_id         uuid not null references periods(id) on delete cascade,
  home_franchise_id uuid not null references franchises(id),
  away_franchise_id uuid references franchises(id)
);
create index matchups_period_idx on matchups (period_id);

-- +goose Down
drop table matchups;
drop table periods;
alter table games drop column home_score, drop column away_score, drop column detail;
