-- +goose Up

-- Each league's waiver order, first claim first. It is set at the start of
-- a season, the commissioner can rearrange it, and a franchise that wins a
-- claim goes to the end. A franchise with no row comes after those with one.
create table waiver_order (
  league_id    uuid not null references leagues(id) on delete cascade,
  franchise_id uuid not null references franchises(id) on delete cascade,
  position     int  not null,
  primary key (league_id, franchise_id)
);

-- A claim says which list the player should land on.
alter table waiver_claims add column list text not null default 'main' check (list in ('main', 'reserve'));

-- Waivers are measured in hours, and every league now has them: any
-- released player waits a day. A league that had chosen blind bids keeps them.
update leagues set settings = jsonb_set(settings, '{waivers}', jsonb_build_object(
  'mode', case when settings->'waivers'->>'mode' = 'faab' then 'faab' else 'rolling' end,
  'hours', 24,
  'budget', coalesce((settings->'waivers'->>'budget')::int, 100)));

-- +goose Down
update leagues set settings = jsonb_set(settings, '{waivers}', jsonb_build_object(
  'mode', settings->'waivers'->>'mode', 'days', 1, 'budget', coalesce((settings->'waivers'->>'budget')::int, 100)));
alter table waiver_claims drop column list;
drop table waiver_order;
