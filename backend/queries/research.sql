-- name: ListResearchSeasons :many
select distinct s.label, s.year
from stat_seasons s
where @competition::text = '' or s.competition = @competition
order by s.year desc, s.label desc;

-- name: ListResearchPlayers :many
-- Latest means the newest imported season in each competition, shared by
-- all players. Aggregate team splits before applying current scoring rules.
with latest as (
  select competition, max(year) as year from stat_seasons group by competition
), totals as (
  select ps.player_id, ps.competition, max(ps.label)::text as season,
         sum(ps.games)::integer as games,
         string_agg(distinct nullif(ps.team, ''), ' / ')::text as season_team
  from player_seasons ps
  join latest on latest.competition = ps.competition
  where ps.league = ''
    and (ps.competition <> 'cbb' or cardinality(@conferences::text[]) = 0 or ps.conference = any(@conferences::text[]))
    and ((@season::text = '' and ps.year = latest.year) or ps.label = @season)
  group by ps.player_id, ps.competition
), stats as (
  select ps.player_id, ps.competition, stat.key, sum(stat.value::numeric)::float8 as value
  from player_seasons ps
  join latest on latest.competition = ps.competition
  cross join lateral jsonb_each_text(ps.stats) as stat(key, value)
  where ps.league = ''
    and (ps.competition <> 'cbb' or cardinality(@conferences::text[]) = 0 or ps.conference = any(@conferences::text[]))
    and ((@season::text = '' and ps.year = latest.year) or ps.label = @season)
  group by ps.player_id, ps.competition, stat.key
), valued as (
  select stats.player_id, stats.competition, jsonb_object_agg(stats.key, stats.value) as stats,
         coalesce(sum(stats.value * (l.settings->'scoring'->>stats.key)::float8), 0)::float8 as points
  from stats left join leagues l on l.competition = stats.competition
  group by stats.player_id, stats.competition
), pool as (
  select p.id, p.competition, p.full_name, p.positions, p.status, p.injury_designation, p.headshot_url,
         coalesce(totals.season_team, t.abbrev, '')::text as team,
         coalesce(f.name, '')::text as owner_name, coalesce(f.slug, '')::text as owner_slug,
         coalesce(totals.season, '')::text as season,
         coalesce(totals.games, 0)::integer as games,
         coalesce(valued.stats, '{}'::jsonb) as stats,
         coalesce(valued.points, 0)::float8 as points,
         coalesce(valued.points / nullif(totals.games, 0), 0)::float8 as points_per_game
  from players p
  left join pro_teams t on t.id = p.pro_team_id
  left join roster_entries r on r.player_id = p.id
  left join franchises f on f.id = r.franchise_id
  left join totals on totals.player_id = p.id and totals.competition = p.competition
  left join valued on valued.player_id = p.id and valued.competition = p.competition
  where (p.competition <> 'cbb' or (totals.player_id is not null))
    and (@competition::text = '' or p.competition = @competition)
    and (@status::text = '' or p.status = @status)
    and (@search::text = '' or p.full_name ilike '%' || @search || '%')
    and (sqlc.narg('available_in')::uuid is null or (
      r.player_id is null and p.competition = (select competition from leagues where id = sqlc.narg('available_in'))))
)
select pool.* from pool
order by
  case when @sort::text = 'name' then full_name end asc,
  case when @sort = 'points' and season <> '' then points
       when @sort = 'points_per_game' and games > 0 then points_per_game
       when @sort = 'games' and season <> '' then games::float8
       else (stats ->> sqlc.arg(sort)::text)::float8 end desc nulls last,
  full_name, id
limit @page_size offset @page_offset;

-- name: ListResearchCatalog :many
select competition, label, year, count(distinct player_id)::integer as players,
       max(synced_at) as synced_at
from player_seasons where league = ''
 and (competition <> 'cbb' or cardinality(@conferences::text[]) = 0 or conference = any(@conferences::text[]))
group by competition, label, year
order by competition, year desc;

-- name: ListResearchSeasonPool :many
-- Historical pools follow the competition where the stats were recorded,
-- even if a player has since changed competitions. Team splits are combined.
with selected as (
 select ps.* from player_seasons ps
 where ps.league = '' and ps.competition = @competition
 and (ps.competition <> 'cbb' or cardinality(@conferences::text[]) = 0 or ps.conference = any(@conferences::text[]))
 and (ps.label = @season::text or (@season = '' and ps.year = (
   select max(year) from stat_seasons where competition = @competition)))
), totals as (
 select player_id, competition, max(label)::text as season,
        sum(games)::integer as games, string_agg(distinct nullif(team,''), ' / ')::text as team
 from selected group by player_id,competition
), stat_values as (
 select player_id, stat.key, sum(stat.value::numeric)::float8 as value
 from selected cross join lateral jsonb_each_text(stats) as stat(key,value)
 group by player_id,stat.key
), stats as (
 select player_id,jsonb_object_agg(key,value) as stats from stat_values group by player_id
)
select p.id, totals.competition, p.full_name, p.positions, p.status, p.injury_designation, p.headshot_url,
       coalesce(totals.team,'')::text as team,
       coalesce(f.name,'')::text as owner_name, coalesce(f.slug,'')::text as owner_slug,
       totals.season, totals.games, coalesce(stats.stats,'{}'::jsonb) as stats,
       0::float8 as points, 0::float8 as points_per_game
from totals join players p on p.id = totals.player_id
left join stats on stats.player_id = p.id
left join roster_entries r on r.player_id = p.id and r.league_id in (select id from leagues where competition = totals.competition)
left join franchises f on f.id = r.franchise_id
order by p.full_name,p.id;
