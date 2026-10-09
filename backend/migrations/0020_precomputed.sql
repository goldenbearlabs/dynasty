-- +goose Up
-- Work the server used to repeat on every read is stored instead: disk is
-- plentiful here, CPU and memory are not. Triggers keep every stored value
-- in step with its source in the same commit, so nothing can read a stale one.

-- ---- indexes for lookups by a column that leads no existing index ----

create index roster_entries_player_idx on roster_entries (player_id);
create index waivers_player_idx on waivers (player_id);
create index transactions_player_idx on transactions (player_id);
create index transactions_dynasty_idx on transactions (dynasty_id, id desc);
create index transactions_acquired_idx on transactions (league_id, franchise_id, created_at);
create index draft_picks_player_idx on draft_picks (player_id);
create index draft_picks_holder_idx on draft_picks (current_franchise_id);
create index trade_items_player_idx on trade_items (player_id);
create index games_starts_idx on games (competition, starts_at);
create index players_team_idx on players (pro_team_id);

-- ---- fantasy points, stored beside the stats they come from ----

-- +goose StatementBegin
create function fantasy_points(stats jsonb, scoring jsonb) returns numeric
language sql immutable parallel safe as $$
  select sum(s.value::numeric * r.value::numeric)
  from jsonb_each_text(stats) s
  join jsonb_each_text(scoring) r on r.key = s.key
$$;
-- +goose StatementEnd

-- Null where no league here plays the sport, or none of its rules apply.
-- eligible_points leaves out a CBB season in a conference that cannot start.
alter table player_seasons add column points float8, add column eligible_points float8;
alter table stat_lines add column points float8;

-- +goose StatementBegin
create function score_stats() returns trigger language plpgsql as $$
declare
  played text;
  rules jsonb;
  conferences jsonb;
begin
  if tg_table_name = 'stat_lines' then
    select competition into played from games where id = new.game_id;
  elsif new.league = '' then
    played := new.competition;
  end if;
  select settings into rules from leagues where competition = played order by id limit 1;
  new.points := fantasy_points(new.stats, rules->'scoring');
  if tg_table_name = 'player_seasons' then
    conferences := coalesce(nullif(rules->'lineup'->'conferences', 'null'::jsonb), '["8","23","7","2","4","44","3","21"]'::jsonb);
    new.eligible_points := case when played <> 'cbb' or jsonb_array_length(conferences) = 0 or conferences ? new.conference
                                then new.points end;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger score_stats before insert or update on player_seasons for each row execute function score_stats();
create trigger score_stats before insert or update on stat_lines for each row execute function score_stats();

-- A league's rules changing rescores its sport. Setting points to itself
-- changes nothing by hand: score_stats recalculates it on the way in.
-- +goose StatementBegin
create function rescore_league() returns trigger language plpgsql as $$
declare
  played text[] := array[]::text[];
begin
  if tg_op <> 'INSERT' then played := played || old.competition; end if;
  if tg_op <> 'DELETE' then played := played || new.competition; end if;
  if tg_op = 'UPDATE' and old.competition = new.competition
     and old.settings->'scoring' is not distinct from new.settings->'scoring'
     and old.settings->'lineup'->'conferences' is not distinct from new.settings->'lineup'->'conferences' then
    return null;
  end if;
  update player_seasons set points = points where competition = any(played) and league = '';
  update stat_lines sl set points = sl.points from games g where g.id = sl.game_id and g.competition = any(played);
  return null;
end;
$$;
-- +goose StatementEnd

create trigger rescore_league after insert or update or delete on leagues for each row execute function rescore_league();

update player_seasons set points = points where league = '';
update stat_lines set points = points;

-- ---- the seasons there are stats for ----

-- A few dozen rows standing in for a scan of every player's history.
create table stat_seasons (
  competition text not null,
  year        int  not null,
  label       text not null,
  primary key (competition, year, label)
);
insert into stat_seasons select distinct competition, year, label from player_seasons where league = '';

-- +goose StatementBegin
create function track_stat_seasons() returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' then
    delete from stat_seasons s
    where not exists (select 1 from player_seasons ps
                      where ps.competition = s.competition and ps.year = s.year and ps.label = s.label and ps.league = '');
  elsif new.league = '' then
    insert into stat_seasons values (new.competition, new.year, new.label) on conflict do nothing;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

create trigger track_stat_seasons after insert or update of label on player_seasons for each row execute function track_stat_seasons();
create trigger forget_stat_seasons after delete on player_seasons for each statement execute function track_stat_seasons();

-- ---- what each franchise scored in a finished head-to-head period ----

-- Written the first time standings need a finished period, read from then
-- on. A period with no rows has not been settled yet.
create table period_scores (
  period_id    uuid   not null references periods(id) on delete cascade,
  franchise_id uuid   not null references franchises(id) on delete cascade,
  points       float8 not null,
  primary key (period_id, franchise_id)
);

-- Anything that could change a settled period's scores unsettles it: a stat
-- line in one of its games, a lineup in force during it, the league's rules.
-- The lock is the one SettlePeriod takes, so a period being settled from
-- the old numbers finishes first and is then cleared.
-- +goose StatementBegin
create function unsettle_periods() returns trigger language plpgsql as $$
declare
  line record;
  stale uuid[];
begin
  if tg_op = 'DELETE' then line := old; else line := new; end if;
  if tg_table_name = 'stat_lines' then
    select array_agg(p.id order by p.id) into stale
    from games g
    join leagues l on l.competition = g.competition
    join seasons s on s.league_id = l.id
    join periods p on p.season_id = s.id and g.day between p.starts_on and p.ends_on
    where g.id = line.game_id;
  elsif tg_table_name = 'leagues' then
    select array_agg(p.id order by p.id) into stale
    from seasons s join periods p on p.season_id = s.id
    where s.league_id = line.id;
  else -- lineups and their entries
    select array_agg(p.id order by p.id) into stale
    from seasons s join periods p on p.season_id = s.id
    where s.league_id = line.league_id and p.ends_on >= line.effective_on;
  end if;
  if stale is not null then
    perform pg_advisory_xact_lock(hashtextextended(id::text, 0)) from unnest(stale) as id;
    delete from period_scores where period_id = any(stale);
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

create trigger unsettle_periods after insert or delete or update of stats, player_id, game_id on stat_lines for each row execute function unsettle_periods();
create trigger unsettle_periods after insert or update or delete on lineups for each row execute function unsettle_periods();
create trigger unsettle_periods after insert or update or delete on lineup_entries for each row execute function unsettle_periods();
create trigger unsettle_periods after update of settings on leagues for each row execute function unsettle_periods();

-- ---- response cache ----

-- Noting that a box score was fetched changes nothing anyone is shown, so
-- it no longer starts a new cache generation with every live poll.
drop trigger invalidate_response_cache on games;
create trigger invalidate_response_cache
  after insert or delete or truncate or update of day, starts_at, status, home_team_id, away_team_id, home_score, away_score, detail
  on games for each statement execute function bump_cache_revision('public');

-- +goose Down
drop trigger invalidate_response_cache on games;
create trigger invalidate_response_cache after insert or update or delete or truncate on games for each statement execute function bump_cache_revision('public');
drop trigger unsettle_periods on leagues;
drop trigger unsettle_periods on lineup_entries;
drop trigger unsettle_periods on lineups;
drop trigger unsettle_periods on stat_lines;
drop function unsettle_periods();
drop table period_scores;
drop trigger forget_stat_seasons on player_seasons;
drop trigger track_stat_seasons on player_seasons;
drop function track_stat_seasons();
drop table stat_seasons;
drop trigger rescore_league on leagues;
drop function rescore_league();
drop trigger score_stats on stat_lines;
drop trigger score_stats on player_seasons;
drop function score_stats();
alter table stat_lines drop column points;
alter table player_seasons drop column points, drop column eligible_points;
drop function fantasy_points(jsonb, jsonb);
drop index players_team_idx;
drop index games_starts_idx;
drop index trade_items_player_idx;
drop index draft_picks_holder_idx;
drop index draft_picks_player_idx;
drop index transactions_acquired_idx;
drop index transactions_dynasty_idx;
drop index transactions_player_idx;
drop index waivers_player_idx;
drop index roster_entries_player_idx;
