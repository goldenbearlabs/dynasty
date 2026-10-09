-- +goose Up
-- Cache generations live in Postgres so an import, background job, direct SQL
-- change or commissioner action invalidates cached reads in the same commit.
-- Live game changes do not invalidate the season-only research generation.
create table cache_revision (
  singleton boolean primary key default true check (singleton),
  namespace uuid not null default gen_random_uuid(),
  research bigint not null default 1,
  public bigint not null default 1,
  research_transaction bigint not null default 0,
  public_transaction bigint not null default 0
);
insert into cache_revision (singleton) values (true);

-- +goose StatementBegin
create function bump_cache_revision() returns trigger language plpgsql as $$
declare
  xid bigint := txid_current();
  season_data boolean := TG_ARGV[0] = 'research';
begin
  -- Once per scope per transaction: bulk imports do not create a new row
  -- version for every player. Generations roll back with the data on failure.
  update cache_revision set
    research = research + case when season_data and research_transaction <> xid then 1 else 0 end,
    research_transaction = case when season_data then xid else research_transaction end,
    public = public + case when public_transaction <> xid then 1 else 0 end,
    public_transaction = xid
  where singleton and (public_transaction <> xid or (season_data and research_transaction <> xid));
  return null;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
do $$
declare tbl text;
begin
  foreach tbl in array array['players','player_seasons','pro_teams','leagues','dynasties','franchises','roster_entries'] loop
    execute format('create trigger invalidate_response_cache after insert or update or delete or truncate on %I for each statement execute function bump_cache_revision(''research'')', tbl);
  end loop;
  foreach tbl in array array['games','stat_lines','seasons','lineups','lineup_entries','periods','matchups','transactions','drafts','draft_leagues','draft_picks','trades','trade_parties','trade_items','waivers','franchise_teams','player_nicknames'] loop
    execute format('create trigger invalidate_response_cache after insert or update or delete or truncate on %I for each statement execute function bump_cache_revision(''public'')', tbl);
  end loop;
end;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
do $$
declare tbl text;
begin
  foreach tbl in array array['players','player_seasons','pro_teams','leagues','dynasties','franchises','roster_entries','games','stat_lines','seasons','lineups','lineup_entries','periods','matchups','transactions','drafts','draft_leagues','draft_picks','trades','trade_parties','trade_items','waivers','franchise_teams','player_nicknames'] loop
    execute format('drop trigger invalidate_response_cache on %I', tbl);
  end loop;
end;
$$;
-- +goose StatementEnd
drop function bump_cache_revision();
drop table cache_revision;
