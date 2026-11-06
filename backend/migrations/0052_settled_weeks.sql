-- +goose Up

-- ---- settled weeks ----

-- What each starter scored in a scoring week that is over, so a season's
-- standings add up stored weeks instead of every box score since opening
-- day. A week is a lineup week, or a doubled matchup as a whole, cut to the
-- season's dates. Written the first time standings need the week, and
-- cleared when anything behind it changes, as period_scores are.
create table settled_weeks (
  league_id uuid not null references leagues(id) on delete cascade,
  starts_on date not null,
  ends_on   date not null,
  primary key (league_id, starts_on)
);

create table week_scores (
  league_id    uuid   not null,
  starts_on    date   not null,
  franchise_id uuid   not null references franchises(id) on delete cascade,
  player_id    uuid   not null references players(id) on delete cascade,
  games        int    not null,
  points       float8 not null,
  primary key (league_id, starts_on, franchise_id, player_id),
  foreign key (league_id, starts_on) references settled_weeks on delete cascade
);

-- Anything that could change a settled week unsettles it: a stat line in
-- one of its games, a game's day or status (on auto, a game counts only
-- once his last is final), a lineup in force during it, a doubled matchup
-- over it, the league's rules, or the season's dates. The lock is the one
-- SettleWeek takes, so a week being settled from the old numbers finishes
-- first and is then cleared.
-- +goose StatementBegin
create function unsettle_weeks() returns trigger language plpgsql as $$
declare
  line record;
  leagues uuid[];
  days date[];
begin
  if tg_op = 'DELETE' then line := old; else line := new; end if;
  if tg_table_name = 'stat_lines' then
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from games g
    join leagues l on l.competition = g.competition
    join settled_weeks w on w.league_id = l.id and g.day between w.starts_on and w.ends_on
    where g.id = line.game_id;
  elsif tg_table_name = 'games' then
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from leagues l join settled_weeks w on w.league_id = l.id
    where l.competition = line.competition
      and (line.day between w.starts_on and w.ends_on or old.day between w.starts_on and w.ends_on);
  elsif tg_table_name = 'periods' then
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from seasons s join settled_weeks w on w.league_id = s.league_id
    where s.id = line.season_id and w.starts_on <= line.ends_on and w.ends_on >= line.starts_on;
  elsif tg_table_name = 'leagues' then
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from settled_weeks w where w.league_id = line.id;
  elsif tg_table_name = 'seasons' then
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from settled_weeks w where w.league_id = line.league_id;
  else -- lineups and their entries
    select array_agg(w.league_id), array_agg(w.starts_on) into leagues, days
    from settled_weeks w where w.league_id = line.league_id and w.ends_on >= line.effective_on;
  end if;
  if leagues is not null then
    perform pg_advisory_xact_lock(hashtextextended(k.league_id::text || k.starts_on::text, 0))
    from (select distinct * from unnest(leagues, days) as u(league_id, starts_on) order by 1, 2) k;
    delete from settled_weeks w using unnest(leagues, days) as u(league_id, starts_on)
    where w.league_id = u.league_id and w.starts_on = u.starts_on;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

create trigger unsettle_weeks after insert or delete or update of stats, player_id, game_id on stat_lines for each row execute function unsettle_weeks();
create trigger unsettle_weeks after update of day, status on games for each row execute function unsettle_weeks();
create trigger unsettle_weeks after insert or update or delete on lineups for each row execute function unsettle_weeks();
create trigger unsettle_weeks after insert or update or delete on lineup_entries for each row execute function unsettle_weeks();
create trigger unsettle_weeks after insert or update or delete on periods for each row execute function unsettle_weeks();
create trigger unsettle_weeks after update of settings on leagues for each row execute function unsettle_weeks();
create trigger unsettle_weeks after update of starts_on, ends_on on seasons for each row execute function unsettle_weeks();

-- ---- live cache generation ----

-- Live games write scores and box scores every poll. They start a new
-- generation of their own, so only what shows live points is read again;
-- rosters, activity and the like stay cached through a night of games.
alter table cache_revision
  add column live bigint not null default 1,
  add column live_transaction bigint not null default 0;

-- +goose StatementBegin
create or replace function bump_cache_revision() returns trigger language plpgsql as $$
declare
  xid bigint := txid_current();
begin
  -- Once per scope per transaction: bulk imports do not create a new row
  -- version for every player. Generations roll back with the data on failure.
  if TG_ARGV[0] = 'live' then
    update cache_revision set live = live + 1, live_transaction = xid
    where singleton and live_transaction <> xid;
    return null;
  end if;
  update cache_revision set
    research = research + case when TG_ARGV[0] = 'research' and research_transaction <> xid then 1 else 0 end,
    research_transaction = case when TG_ARGV[0] = 'research' then xid else research_transaction end,
    public = public + case when public_transaction <> xid then 1 else 0 end,
    public_transaction = xid
  where singleton and (public_transaction <> xid or (TG_ARGV[0] = 'research' and research_transaction <> xid));
  return null;
end;
$$;
-- +goose StatementEnd

drop trigger invalidate_response_cache on games;
create trigger invalidate_response_cache
  after insert or delete or truncate or update of day, starts_at, status, home_team_id, away_team_id, home_score, away_score, detail
  on games for each statement execute function bump_cache_revision('live');
drop trigger invalidate_response_cache on stat_lines;
create trigger invalidate_response_cache after insert or update or delete or truncate on stat_lines
  for each statement execute function bump_cache_revision('live');

-- +goose Down
drop trigger invalidate_response_cache on stat_lines;
create trigger invalidate_response_cache after insert or update or delete or truncate on stat_lines
  for each statement execute function bump_cache_revision('public');
drop trigger invalidate_response_cache on games;
create trigger invalidate_response_cache
  after insert or delete or truncate or update of day, starts_at, status, home_team_id, away_team_id, home_score, away_score, detail
  on games for each statement execute function bump_cache_revision('public');
-- +goose StatementBegin
create or replace function bump_cache_revision() returns trigger language plpgsql as $$
declare
  xid bigint := txid_current();
  season_data boolean := TG_ARGV[0] = 'research';
begin
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
alter table cache_revision drop column live_transaction, drop column live;
drop trigger unsettle_weeks on seasons;
drop trigger unsettle_weeks on leagues;
drop trigger unsettle_weeks on periods;
drop trigger unsettle_weeks on lineup_entries;
drop trigger unsettle_weeks on lineups;
drop trigger unsettle_weeks on games;
drop trigger unsettle_weeks on stat_lines;
drop function unsettle_weeks();
drop table week_scores;
drop table settled_weeks;
