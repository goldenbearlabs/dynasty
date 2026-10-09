-- +goose Up
alter table franchises add column image_url text not null default '';

-- A franchise may brand each sport differently without renaming the shared league.
create table franchise_teams (
 franchise_id uuid not null references franchises(id) on delete cascade,
 league_id uuid not null references leagues(id) on delete cascade,
 name text not null default '',
 image_url text not null default '',
 primary key (franchise_id, league_id)
);
-- Nicknames are an organization's own labels; provider names remain canonical.
create table player_nicknames (
 franchise_id uuid not null references franchises(id) on delete cascade,
 player_id uuid not null references players(id) on delete cascade,
 nickname text not null,
 primary key (franchise_id, player_id)
);

-- +goose Down
drop table player_nicknames;
drop table franchise_teams;
alter table franchises drop column image_url;
