-- +goose Up

-- A period whose matchups the commissioner set by hand. A rebuilt schedule
-- keeps them, and the playoffs do not seed over them.
alter table periods add column by_hand boolean not null default false;

-- +goose Down
alter table periods drop column by_hand;
