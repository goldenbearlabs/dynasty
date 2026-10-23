-- +goose Up
-- The betting market's current view of a game, overwritten each time it is
-- read: each side's implied score and the home side's chance of winning.
-- Null until a line is posted. No history is kept.
alter table games add column home_implied float8;
alter table games add column away_implied float8;
alter table games add column home_win float8;

-- +goose Down
alter table games drop column home_win;
alter table games drop column away_implied;
alter table games drop column home_implied;
