-- +goose Up

-- When a manager last sent this player, not a prospect, to the reserve
-- list. A league's reserve lock counts from here. Null on the main roster,
-- and for players who are on reserve by rule (prospects, graduates) or by
-- the commissioner's hand.
alter table roster_entries add column reserved_at timestamptz;

-- +goose Down
alter table roster_entries drop column reserved_at;
