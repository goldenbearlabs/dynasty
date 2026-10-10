-- +goose Up

-- Drafted in his league's startup draft: he may sit on the reserve list
-- whatever the league's rule for it, so a startup draft can fill both lists.
alter table roster_entries add column startup boolean not null default false;
update roster_entries r set startup = true
from draft_picks k join drafts d on d.id = k.draft_id
where d.kind = 'startup' and k.player_id = r.player_id and k.league_id = r.league_id
  and r.acquired_via in ('draft', 'trade');

-- Existing leagues get the season lock: nobody is called up from the
-- reserve list while a season is being played.
update leagues set settings = jsonb_set(settings, '{roster,reserve_lock_season}', 'true')
where not (settings->'roster' ? 'reserve_lock_season');

-- +goose Down
update leagues set settings = settings #- '{roster,reserve_lock_season}';
alter table roster_entries drop column startup;
