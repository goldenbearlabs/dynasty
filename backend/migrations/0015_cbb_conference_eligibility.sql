-- +goose Up
alter table pro_teams add column conference text not null default '';
alter table player_seasons add column conference text not null default '';

-- Existing leagues adopt the same defaults as newly created leagues.
update leagues set settings = jsonb_set(settings, '{lineup,conferences}',
  '["8","23","7","2","4","44","3","21"]'::jsonb)
where competition = 'cbb' and not (settings->'lineup' ? 'conferences');
update leagues set settings = jsonb_set(settings, '{roster,reserve_eligibility}',
  '"prospects_or_ineligible"'::jsonb)
where competition = 'cbb' and settings->'roster'->>'reserve_eligibility' = 'prospects_only';

-- +goose Down
update leagues set settings = settings #- '{lineup,conferences}'
where competition = 'cbb';
update leagues set settings = jsonb_set(settings, '{roster,reserve_eligibility}', '"prospects_only"'::jsonb)
where competition = 'cbb' and settings->'roster'->>'reserve_eligibility' = 'prospects_or_ineligible';
alter table player_seasons drop column conference;
alter table pro_teams drop column conference;
