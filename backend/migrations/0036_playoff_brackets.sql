-- +goose Up
alter table matchups
  add column bracket text not null default 'championship' check (bracket in ('championship','consolation')),
  add column bracket_slot int not null default 0 check (bracket_slot >= 0);

with ordered as (
 select id,(row_number() over(partition by period_id,bracket order by id)-1)::int as slot from matchups
) update matchups m set bracket_slot=o.slot from ordered o where m.id=o.id;

-- Upgrade the former six-team default; retain other configured playoff sizes.
update leagues set settings=jsonb_set(settings,'{format}',(settings->'format') || jsonb_build_object(
  'playoff_teams',case when (settings->'format'->>'playoff_teams')::int=6 then 8 else (settings->'format'->>'playoff_teams')::int end,
  'standings_by','record',
  'tiebreakers',jsonb_build_array('points_for','head_to_head','points_against'),
  'reseed',false,
  'consolation',true,
  'consolation_seeding','bottom_first'
)) where not (settings->'format' ? 'tiebreakers');

-- +goose Down
update leagues set settings=jsonb_set(settings,'{format}',(settings->'format') - 'standings_by' - 'tiebreakers' - 'reseed' - 'consolation' - 'consolation_seeding');
alter table matchups drop column bracket, drop column bracket_slot;
