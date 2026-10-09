-- ---- games and stat lines ----

-- name: FindProTeam :one
select id from pro_teams where competition = @competition and provider_id = @provider_id;

-- name: UpsertGame :exec
insert into games (competition, provider_id, day, starts_at, status, home_team_id, away_team_id, home_score, away_score, detail)
values (@competition, @provider_id, @day, @starts_at, @status, @home_team_id, @away_team_id, @home_score, @away_score, @detail)
on conflict (competition, provider_id) do update
  set day = excluded.day, starts_at = excluded.starts_at, status = excluded.status,
      home_team_id = excluded.home_team_id, away_team_id = excluded.away_team_id,
      home_score = excluded.home_score, away_score = excluded.away_score, detail = excluded.detail;

-- name: ListGamesInPlay :many
-- Games that are live, or should have started: the ones worth polling.
select * from games
where competition = @competition
  and (status = 'live'
       or (status = 'scheduled' and starts_at between now() - interval '8 hours' and now() + interval '2 minutes'))
order by starts_at;

-- name: GetGame :one
select * from games where id = @id;

-- name: ListGamesNeedingStats :many
-- Games to fetch a box score for: every live game, every finished game not
-- yet fetched, and each finished game once more the next day, when stat
-- corrections have been made.
select * from games
where competition = @competition
  and starts_at > now() - interval '4 days'
  and (status = 'live'
       or (status = 'final' and (stats_synced_at is null
           or (stats_synced_at < starts_at + interval '20 hours' and now() > starts_at + interval '20 hours'))))
order by starts_at;

-- name: UpsertStatLine :exec
insert into stat_lines (game_id, player_id, stats) values (@game_id, @player_id, @stats)
on conflict (game_id, player_id) do update set stats = excluded.stats;

-- name: MarkGameStatsSynced :exec
update games set stats_synced_at = now() where id = @id;

-- ---- seasons ----

-- name: CreateSeason :one
insert into seasons (league_id, year, starts_on, ends_on)
values (@league_id, @year, @starts_on, @ends_on)
returning *;

-- name: GetSeason :one
select * from seasons where id = @id;

-- name: LatestSeason :one
select * from seasons where league_id = @league_id order by year desc limit 1;

-- name: ListSeasons :many
select s.* from seasons s
join leagues l on l.id = s.league_id
where l.dynasty_id = @dynasty_id
order by s.year desc;

-- name: SetSeasonDates :exec
update seasons set starts_on = @starts_on, ends_on = @ends_on where id = @id;

-- name: CompleteSeason :exec
update seasons set status = 'complete', champion_franchise_id = @champion_franchise_id where id = @id;

-- name: ReopenSeason :exec
update seasons set status = 'active', champion_franchise_id = null where id = @id;

-- ---- lineups ----

-- name: LineupInForce :one
-- The day of the lineup that applies on a given day, if any has been set.
select max(effective_on)::date as effective_on from lineups
where league_id = @league_id and franchise_id = @franchise_id and effective_on <= @day;

-- name: ListLineupEntries :many
select slot, slot_index, player_id, counts_from from lineup_entries
where league_id = @league_id and franchise_id = @franchise_id and effective_on = @effective_on
order by slot, slot_index;

-- name: DeleteLineups :exec
-- Removes the lineups that take effect within a span of days.
delete from lineups
where league_id = @league_id and franchise_id = @franchise_id and effective_on between @from_day and @to_day;

-- name: InsertLineup :exec
insert into lineups (league_id, franchise_id, effective_on) values (@league_id, @franchise_id, @effective_on)
on conflict do nothing;

-- name: InsertLineupEntry :exec
insert into lineup_entries (league_id, franchise_id, effective_on, slot, slot_index, player_id, counts_from)
values (@league_id, @franchise_id, @effective_on, @slot, @slot_index, @player_id, @counts_from);

-- name: CopyLineupEntries :exec
insert into lineup_entries (league_id, franchise_id, effective_on, slot, slot_index, player_id, counts_from)
select e.league_id, e.franchise_id, @to_day, e.slot, e.slot_index, e.player_id, e.counts_from
from lineup_entries e
where e.league_id = @league_id and e.franchise_id = @franchise_id and e.effective_on = @from_day;

-- name: RemoveFromLineups :exec
-- Takes a player out of every lineup that takes effect on or after a day.
delete from lineup_entries
where league_id = @league_id and franchise_id = @franchise_id and player_id = @player_id and effective_on >= @from_day;

-- name: PlayerHasStartedGame :one
-- Whether the player's real team has a game on the day that has started.
select exists (
  select 1 from games g
  join players p on p.pro_team_id in (g.home_team_id, g.away_team_id)
  where p.id = @player_id and g.competition = p.competition and g.day = @day and g.starts_at <= now()
);

-- name: ListRosterGames :many
-- A franchise's main roster in one league with each player's games in a
-- span of days and the fantasy points he scored in each. A player with no
-- game in the span appears once, with no game.
select p.id as player_id, p.full_name, p.positions, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev, coalesce(t.conference, '')::text as conference,
       g.id as game_id, g.day as game_day, g.starts_at, coalesce(g.status, '')::text as game_status,
       coalesce(case when g.home_team_id = p.pro_team_id then away.abbrev else home.abbrev end, '')::text as opponent,
       coalesce(g.home_team_id = p.pro_team_id, false)::boolean as at_home,
       coalesce((select sum(s.value::numeric * r.value::numeric)
                 from stat_lines sl
                 cross join lateral jsonb_each_text(sl.stats) s
                 join lateral jsonb_each_text(l.settings->'scoring') r on r.key = s.key
                 where sl.game_id = g.id and sl.player_id = p.id), 0)::float8 as points
from roster_entries re
join players p on p.id = re.player_id
join leagues l on l.id = re.league_id
left join pro_teams t on t.id = p.pro_team_id
left join games g on g.competition = l.competition
                 and g.day between @from_day and @to_day
                 and p.pro_team_id in (g.home_team_id, g.away_team_id)
left join pro_teams home on home.id = g.home_team_id
left join pro_teams away on away.id = g.away_team_id
where re.league_id = @league_id and re.franchise_id = @franchise_id and re.list = 'main'
order by p.full_name, p.id, g.starts_at;

-- ---- points ----

-- name: ListLineupPoints :many
-- Fantasy points for a league over a span of days, by franchise and player:
-- each day's stat lines for whoever was in the lineup in force that day,
-- weighted by the league's scoring rules. A slot may count only some of a
-- player's games each week (games_per_week): those are his first so many on
-- or after the day his manager picked. A league may also cap a team's
-- pitcher starts for the week (pitcher_starts_per_week): starts beyond it,
-- in the order they were played, score nothing. Weeks are counted from
-- week_start, the first day of the week from_day falls in, so a span that
-- begins mid-week still knows which games came earlier.
with days as (
  select generate_series(@week_start::date, @to_day::date, interval '1 day')::date as day
), in_force as (
  select f.id as franchise_id, d.day,
         (select max(n.effective_on) from lineups n
          where n.league_id = @league_id and n.franchise_id = f.id and n.effective_on <= d.day) as effective_on
  from franchises f
  cross join days d
  where f.dynasty_id = (select dynasty_id from leagues where id = @league_id)
), started as (
  select i.franchise_id, e.player_id, g.id as game_id, g.day, sl.stats, l.settings,
         coalesce((select (def.rule->>'games_per_week')::int
                   from jsonb_array_elements(l.settings->'lineup'->'slots') as def(rule)
                   where def.rule->>'name' = e.slot), 0) as games_per_week,
         row_number() over (partition by i.franchise_id, e.player_id, (g.day - @week_start::date) / 7
                            order by g.starts_at, g.id) as nth,
         coalesce((l.settings->'lineup'->>'pitcher_starts_per_week')::int, 0) as starts_allowed,
         -- A start, for the cap: he started the game, and if he is a reliever
         -- he went more than an inning, so an opener does not use one up.
         (coalesce((sl.stats->>'pit_gs')::numeric, 0) >= 1
          and (not ('RP' = any(pl.positions)) or coalesce((sl.stats->>'pit_ip')::numeric, 0) > 1)) as is_start,
         sum(case when coalesce((sl.stats->>'pit_gs')::numeric, 0) >= 1
                   and (not ('RP' = any(pl.positions)) or coalesce((sl.stats->>'pit_ip')::numeric, 0) > 1)
                  then 1 else 0 end)
           over (partition by i.franchise_id, (g.day - @week_start::date) / 7
                 order by g.starts_at, g.id, e.player_id) as starts_so_far
  from in_force i
  join lineup_entries e on e.league_id = @league_id and e.franchise_id = i.franchise_id and e.effective_on = i.effective_on
  join players pl on pl.id = e.player_id
  join leagues l on l.id = @league_id
  join games g on g.competition = l.competition and g.day = i.day
  join stat_lines sl on sl.game_id = g.id and sl.player_id = e.player_id
  where (e.counts_from is null or g.day >= e.counts_from)
    and (l.competition <> 'cbb'
      or jsonb_array_length(coalesce(nullif(l.settings->'lineup'->'conferences', 'null'::jsonb), '["8","23","7","2","4","44","3","21"]'::jsonb)) = 0
      or coalesce(
        (select ps.conference from player_seasons ps
         join pro_teams team on team.competition = ps.competition and team.abbrev = ps.team
         where ps.player_id = sl.player_id and ps.competition = 'cbb' and ps.league = ''
           and ps.year = extract(year from g.day)::int + case when extract(month from g.day) >= 7 then 1 else 0 end
           and team.id in (g.home_team_id, g.away_team_id) and ps.conference <> '' limit 1),
        (select team.conference from players p join pro_teams team on team.id = p.pro_team_id where p.id = sl.player_id), '')
         in (select jsonb_array_elements_text(coalesce(nullif(l.settings->'lineup'->'conferences', 'null'::jsonb), '["8","23","7","2","4","44","3","21"]'::jsonb))))
)
select st.franchise_id, st.player_id, p.full_name, p.headshot_url,
       count(distinct st.game_id) as games,
       sum(s.value::numeric * r.value::numeric)::float8 as points
from started st
join players p on p.id = st.player_id
cross join lateral jsonb_each_text(st.stats) s
join lateral jsonb_each_text(st.settings->'scoring') r on r.key = s.key
where st.day >= @from_day::date
  and (st.games_per_week = 0 or st.nth <= st.games_per_week)
  and (st.starts_allowed = 0 or not st.is_start or st.starts_so_far <= st.starts_allowed)
group by st.franchise_id, st.player_id, p.full_name, p.headshot_url
order by points desc;

-- name: ListWeekStarts :many
-- The pitcher starts one franchise's starters have made in a week, in the
-- order they were played. See ListLineupPoints for what counts as one.
select pl.id as player_id, pl.full_name, g.day, g.starts_at,
       coalesce((sl.stats->>'pit_ip')::numeric, 0)::float8 as innings
from generate_series(@week_start::date, @week_end::date, interval '1 day') as d(day)
join lateral (
  select max(n.effective_on) as effective_on from lineups n
  where n.league_id = @league_id and n.franchise_id = @franchise_id and n.effective_on <= d.day::date
) in_force on true
join lineup_entries e on e.league_id = @league_id and e.franchise_id = @franchise_id and e.effective_on = in_force.effective_on
join players pl on pl.id = e.player_id
join leagues l on l.id = @league_id
join games g on g.competition = l.competition and g.day = d.day::date
join stat_lines sl on sl.game_id = g.id and sl.player_id = e.player_id
where coalesce((sl.stats->>'pit_gs')::numeric, 0) >= 1
  and (not ('RP' = any(pl.positions)) or coalesce((sl.stats->>'pit_ip')::numeric, 0) > 1)
order by g.starts_at, g.id, e.player_id;

-- ---- merging players ----

-- name: MoveStatLines :exec
-- Keeps the surviving player's own line where both have one for a game.
update stat_lines sl set player_id = @keep_id
where sl.player_id = @duplicate_id
  and not exists (select 1 from stat_lines k where k.game_id = sl.game_id and k.player_id = @keep_id);

-- name: MoveLineupEntries :exec
update lineup_entries set player_id = @keep_id where player_id = @duplicate_id;

-- ---- scoreboards and game pages ----

-- name: ListGamesOnDay :many
select g.id, g.competition, g.day, g.starts_at, g.status, g.detail, g.home_score, g.away_score,
       coalesce(h.abbrev, '')::text   as home_abbrev,
       coalesce(h.name, '')::text     as home_name,
       coalesce(h.logo_url, '')::text as home_logo,
       coalesce(a.abbrev, '')::text   as away_abbrev,
       coalesce(a.name, '')::text     as away_name,
       coalesce(a.logo_url, '')::text as away_logo
from games g
left join pro_teams h on h.id = g.home_team_id
left join pro_teams a on a.id = g.away_team_id
where g.competition = @competition and g.day = @day
order by g.starts_at, g.id;

-- name: GetGameSummary :one
select g.id, g.competition, g.day, g.starts_at, g.status, g.detail, g.home_score, g.away_score,
       coalesce(h.abbrev, '')::text   as home_abbrev,
       coalesce(h.name, '')::text     as home_name,
       coalesce(h.logo_url, '')::text as home_logo,
       coalesce(a.abbrev, '')::text   as away_abbrev,
       coalesce(a.name, '')::text     as away_name,
       coalesce(a.logo_url, '')::text as away_logo
from games g
left join pro_teams h on h.id = g.home_team_id
left join pro_teams a on a.id = g.away_team_id
where g.id = @id;

-- name: ListGameLines :many
-- Every player's line in a game, with the fantasy points it is worth under
-- the league that plays this sport and the franchise that has him, if any.
select p.id as player_id, p.full_name, p.positions, p.headshot_url,
       coalesce(p.pro_team_id = g.home_team_id, false)::boolean as at_home,
       sl.stats,
       coalesce((select sum(s.value::numeric * r.value::numeric)
                 from jsonb_each_text(sl.stats) s
                 join lateral jsonb_each_text(l.settings->'scoring') r on r.key = s.key), 0)::float8 as points,
       coalesce(f.name, '')::text as owner_name,
       coalesce(f.slug, '')::text as owner_slug
from stat_lines sl
join games g on g.id = sl.game_id
join players p on p.id = sl.player_id
left join leagues l on l.competition = g.competition
left join roster_entries re on re.league_id = l.id and re.player_id = p.id
left join franchises f on f.id = re.franchise_id
where sl.game_id = @game_id
order by points desc, p.full_name;

-- ---- head-to-head ----

-- name: ListPeriods :many
select * from periods where season_id = @season_id order by seq;

-- name: GetPeriod :one
select * from periods where id = @id;

-- name: InsertPeriod :one
insert into periods (season_id, seq, starts_on, ends_on, is_playoff)
values (@season_id, @seq, @starts_on, @ends_on, @is_playoff)
returning *;

-- name: DeletePeriodsFrom :exec
-- Removes the periods that start on or after a day, with their matchups.
delete from periods where season_id = @season_id and starts_on >= @from_day;

-- name: InsertMatchup :exec
insert into matchups (period_id, home_franchise_id, away_franchise_id)
values (@period_id, @home_franchise_id, @away_franchise_id);

-- name: ListSeasonMatchups :many
select m.*, p.seq, p.is_playoff from matchups m
join periods p on p.id = m.period_id
where p.season_id = @season_id
order by p.seq, m.id;

-- name: GetMatchup :one
select * from matchups where id = @id;

-- name: ListActiveSeasons :many
select * from seasons where status = 'active';
