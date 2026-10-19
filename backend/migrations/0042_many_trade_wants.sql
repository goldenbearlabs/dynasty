-- +goose Up
-- A manager may say several things they are looking for in one league,
-- each with its own positions and note.
alter table trade_wants add column id uuid not null default gen_random_uuid();
alter table trade_wants drop constraint trade_wants_pkey;
alter table trade_wants add primary key (id);
create index trade_wants_franchise_idx on trade_wants (franchise_id, league_id);

-- +goose Down
-- Keeps each manager's latest want in each league.
delete from trade_wants w using trade_wants newer
where newer.franchise_id = w.franchise_id and newer.league_id = w.league_id
  and (newer.updated_at, newer.id) > (w.updated_at, w.id);
drop index trade_wants_franchise_idx;
alter table trade_wants drop constraint trade_wants_pkey;
alter table trade_wants drop column id;
alter table trade_wants add primary key (franchise_id, league_id);
