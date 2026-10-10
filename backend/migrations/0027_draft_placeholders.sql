-- +goose Up
alter table drafts add column is_placeholder boolean not null default false;
-- Existing untouched future rookie drafts follow the automatic naming convention.
update drafts d set is_placeholder = true
where d.kind = 'seasonal' and d.status = 'scheduled'
 and d.year > extract(year from current_date)
 and exists (select 1 from draft_leagues dl join leagues l on l.id = dl.league_id
             where dl.draft_id = d.id and d.name = d.year::text || ' ' || l.name || ' Draft')
 and not exists (select 1 from draft_picks p where p.draft_id = d.id and p.player_id is not null);

-- +goose Down
alter table drafts drop column is_placeholder;
