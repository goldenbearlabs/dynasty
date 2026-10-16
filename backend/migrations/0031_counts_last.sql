-- +goose Up
-- In a slot that counts only some games a week, a starter left on "auto"
-- counts his last games of the week with stats, rather than the games his
-- manager picked. Existing entries keep the games they picked.
alter table lineup_entries add column counts_last boolean not null default false;

-- +goose Down
alter table lineup_entries drop column counts_last;
