-- +goose Up
alter table players add column injury_designation text not null default '';
alter table players add column injury_checked_at timestamptz;

-- +goose Down
alter table players drop column injury_checked_at;
alter table players drop column injury_designation;
