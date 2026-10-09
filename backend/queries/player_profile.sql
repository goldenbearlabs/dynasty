-- name: ListPlayerTimeline :many
select x.id, x.kind, x.detail, x.created_at,
       coalesce(l.competition, '')::text as competition,
       coalesce(f.name, '')::text as franchise_name, coalesce(f.slug, '')::text as franchise_slug,
       x.trade_id, x.draft_pick_id, coalesce(d.id, null)::uuid as draft_id,
       coalesce(d.name, '')::text as draft_name, coalesce(k.round, 0)::int as pick_round,
       coalesce(k.position, 0)::int as pick_position
from transactions x
left join leagues l on l.id = x.league_id
left join franchises f on f.id = x.franchise_id
left join draft_picks k on k.id = x.draft_pick_id
left join drafts d on d.id = k.draft_id
where x.player_id = @player_id
order by x.id desc limit @page_size offset @page_offset;

-- name: CountPlayerTimeline :one
select count(*) from transactions where player_id = @player_id;

-- name: ListPlayerGameLog :many
select g.id, g.competition, g.day, g.starts_at, sl.stats,
       coalesce(a.abbrev, '')::text as away_abbrev, coalesce(h.abbrev, '')::text as home_abbrev
from stat_lines sl join games g on g.id = sl.game_id
left join pro_teams a on a.id = g.away_team_id
left join pro_teams h on h.id = g.home_team_id
where sl.player_id = @player_id and g.status = 'final'
  and (@competition::text = '' or g.competition = @competition)
order by g.starts_at desc, g.id desc limit @page_size offset @page_offset;

-- name: CountPlayerGameLog :one
select count(*) from stat_lines sl join games g on g.id = sl.game_id
where sl.player_id = @player_id and g.status = 'final'
  and (@competition::text = '' or g.competition = @competition);

-- name: ListPlayerOwnership :many
select r.league_id, l.competition, l.name as league_name,
       f.name as franchise_name, f.slug as franchise_slug, f.id as franchise_id,
       r.list, r.acquired_via, r.acquired_at, r.reserved_at,
       coalesce(case when r.list = 'main' then (select e.slot from lineup_entries e
         where e.league_id = r.league_id and e.franchise_id = r.franchise_id and e.player_id = r.player_id
           and e.effective_on = (select max(n.effective_on) from lineups n
             where n.league_id = r.league_id and n.franchise_id = r.franchise_id and n.effective_on <= @day::date)) end, '')::text as slot
from roster_entries r join leagues l on l.id = r.league_id
join franchises f on f.id = r.franchise_id
where r.player_id = @player_id order by l.competition;

-- name: ListPlayerDraftRecord :many
select k.id, k.round, k.position, k.auto_picked, k.picked_at,
       d.id as draft_id, d.name as draft_name, d.year, d.kind,
       f.name as franchise_name, f.slug as franchise_slug, coalesce(l.competition, '')::text as competition
from draft_picks k join drafts d on d.id = k.draft_id
join franchises f on f.id = k.current_franchise_id
left join leagues l on l.id = k.league_id
where k.player_id = @player_id order by k.picked_at desc, k.position;

-- name: ListPlayerTradeRecord :many
select t.id, t.status, t.note, t.created_at, t.resolved_at,
       f.name as from_name, f.slug as from_slug, dest.name as to_name, dest.slug as to_slug,
       l.competition
from trade_items i join trades t on t.id = i.trade_id
join leagues l on l.id = i.league_id
join franchises f on f.id = i.from_franchise
join franchises dest on dest.id = i.to_franchise
where i.player_id = @player_id and t.status in ('executed', 'reversed')
order by t.created_at desc;

-- name: ListPlayerWaiverStatus :many
select w.league_id, l.competition, w.clears_at
from waivers w join leagues l on l.id = w.league_id
where w.player_id = @player_id;
