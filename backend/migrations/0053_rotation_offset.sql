-- +goose Up
-- How far the commissioner has moved a season's round robin on: period N
-- plays round N + offset. Continuing the pattern from a week set by hand
-- sets it, so the rounds after it follow on, and a rebuild keeps them.
alter table seasons add column rotation_offset int not null default 0;

-- +goose Down
alter table seasons drop column rotation_offset;
