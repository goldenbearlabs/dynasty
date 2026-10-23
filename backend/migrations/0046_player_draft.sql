-- +goose Up
-- Where a player was drafted into his league: the year, the round, his
-- place in that round, and overall. Empty for anyone undrafted or from a
-- feed that does not say.
alter table players
  add column draft_year    int,
  add column draft_round   int,
  add column draft_pick    int,
  add column draft_overall int;

-- +goose Down
alter table players
  drop column draft_year,
  drop column draft_round,
  drop column draft_pick,
  drop column draft_overall;
