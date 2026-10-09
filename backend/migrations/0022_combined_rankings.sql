-- +goose Up
alter table rankings alter column league_id drop not null;
-- A combined board belongs to its startup draft and follows its lifecycle.
alter table rankings add constraint rankings_scope check (league_id is not null or draft_id is not null);
alter table rankings drop constraint rankings_draft_id_fkey;
alter table rankings add foreign key (draft_id) references drafts(id) on delete cascade;

-- +goose Down
delete from rankings where league_id is null;
alter table rankings drop constraint rankings_scope;
alter table rankings alter column league_id set not null;
alter table rankings drop constraint rankings_draft_id_fkey;
alter table rankings add foreign key (draft_id) references drafts(id) on delete set null;
