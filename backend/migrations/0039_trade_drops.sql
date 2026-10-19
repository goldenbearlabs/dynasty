-- +goose Up
alter table trades add column drops jsonb not null default '[]'::jsonb;

-- +goose Down
alter table trades drop column drops;
