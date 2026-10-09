-- +goose Up

-- The NFL league now holds only quarterbacks, running backs, fullbacks,
-- receivers and tight ends, so a lineup slot nobody left can fill (the
-- kicker's) is dropped, with the scoring for stats nobody left can record.
update leagues set settings = jsonb_set(
  settings #- '{scoring,fg_made}' #- '{scoring,xp_made}',
  '{lineup,slots}',
  coalesce((select jsonb_agg(slot order by n)
            from jsonb_array_elements(settings->'lineup'->'slots') with ordinality as slots(slot, n)
            where slot->'positions' ?| array['QB', 'RB', 'FB', 'WR', 'TE', '*']), '[]'::jsonb))
where competition = 'nfl';

-- +goose Down
