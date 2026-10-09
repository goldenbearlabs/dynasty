-- +goose Up

-- For a starter in a slot that counts only some of his games each week: the
-- day his counted games begin, which is how a manager picks the game. Null
-- means from the start of the week.
alter table lineup_entries add column counts_from date;

-- +goose Down
alter table lineup_entries drop column counts_from;
