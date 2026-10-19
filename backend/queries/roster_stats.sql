-- name: ListRosterStatsPool :many
-- Performance is all recorded production, independent of fantasy lineups or
-- current ownership. Read the full competition before calculating ranks.
with lines as (
 select ps.player_id, ps.games::bigint as games, ps.stats
 from player_seasons ps
 where @use_season::boolean and ps.competition = @competition and ps.league = ''
   and ps.year = @year and ps.label = @season
 union all
 select sl.player_id, 1::bigint as games, sl.stats
 from games g join stat_lines sl on sl.game_id = g.id
 where not @use_season::boolean and g.competition = @competition
   and g.day between @from_day::date and @to_day::date
   and g.status in ('live','final')
   and exists (select 1 from jsonb_each_text(sl.stats) v
               where v.value ~ '^-?[0-9.]+$' and v.value::numeric <> 0)
), played as (
 select player_id, sum(games)::bigint as games from lines group by player_id
), stat_values as (
 select player_id, stat.key as stat_key, sum(stat.value::numeric)::float8 as stat_value
 from lines cross join lateral jsonb_each_text(lines.stats) as stat(key,value)
 where stat.value ~ '^-?[0-9.]+$'
 group by player_id, stat_key
), totals as (
 select player_id, jsonb_object_agg(stat_key,stat_value) as stats from stat_values group by player_id
)
select p.id as player_id, p.full_name, p.positions, played.games,
       coalesce(totals.stats,'{}'::jsonb) as stats
from played join players p on p.id = played.player_id
left join totals on totals.player_id = p.id
order by p.id;
