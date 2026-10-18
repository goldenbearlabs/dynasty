-- +goose Up
-- When the commissioner wants every season roster in: a franchise that has
-- not set its roster by then is held to its lists as they stand.
alter table seasons add column roster_deadline timestamptz;

-- +goose Down
alter table seasons drop column roster_deadline;
