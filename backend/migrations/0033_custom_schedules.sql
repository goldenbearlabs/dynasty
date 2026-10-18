-- +goose Up
create table custom_schedules (
  season_id uuid primary key references seasons(id) on delete cascade
);

-- +goose Down
drop table custom_schedules;
