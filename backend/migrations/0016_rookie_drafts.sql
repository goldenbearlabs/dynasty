-- +goose Up

-- A player picked in a rookie draft is held as "rights": the franchise owns
-- him and nobody else can add him, but he is on neither list and counts
-- against no limit. He has until rights_until to be signed to the reserve
-- list; after it he is released. rights_until is set when the draft ends.
alter table roster_entries drop constraint roster_entries_list_check;
alter table roster_entries add constraint roster_entries_list_check check (list in ('main', 'reserve', 'rights'));
alter table roster_entries add column rights_until timestamptz;
-- Signed from a rookie draft: he may sit on the reserve list whatever the
-- league's rule for it, until he is moved to the main roster.
alter table roster_entries add column rookie boolean not null default false;

-- A pick its owner chose not to use. Unlike a pick skipped by the clock, it
-- cannot be made up later.
alter table draft_picks add column passed_at timestamptz;

-- Existing leagues get the signing window. Their rounds stay as they were set.
update leagues set settings = jsonb_set(settings, '{draft,signing_days}', '7') where not (settings->'draft' ? 'signing_days');

-- +goose Down
update leagues set settings = settings #- '{draft,signing_days}';
alter table draft_picks drop column passed_at;
delete from roster_entries where list = 'rights';
alter table roster_entries drop column rookie;
alter table roster_entries drop column rights_until;
alter table roster_entries drop constraint roster_entries_list_check;
alter table roster_entries add constraint roster_entries_list_check check (list in ('main', 'reserve'));
