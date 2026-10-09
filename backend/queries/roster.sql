-- name: LockFranchiseRoster :exec
-- Serializes roster changes for one franchise in one league until the
-- surrounding transaction ends.
select pg_advisory_xact_lock(hashtextextended(@league_id::text || @franchise_id::text, 0));

-- name: ListRosterEntries :many
-- The facts roster limits depend on.
select r.player_id, r.list, p.status, coalesce(t.conference, '')::text as conference
from roster_entries r
join players p on p.id = r.player_id
left join pro_teams t on t.id = p.pro_team_id
where r.league_id = @league_id and r.franchise_id = @franchise_id;

-- name: ListFranchiseRoster :many
-- Every player a franchise holds, across all its leagues, for display.
select r.league_id, r.list, r.acquired_via, r.acquired_at, r.reserved_at,
       p.id as player_id, p.full_name, p.positions, p.status, p.class, p.note, p.birth_date, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev
from roster_entries r
join players p on p.id = r.player_id
left join pro_teams t on t.id = p.pro_team_id
where r.franchise_id = @franchise_id
order by p.full_name;

-- name: InsertRosterEntry :exec
insert into roster_entries (league_id, franchise_id, player_id, list, acquired_via, reserved_at)
values (@league_id, @franchise_id, @player_id, @list, @acquired_via, @reserved_at);

-- name: GetRosterEntry :one
select * from roster_entries where league_id = @league_id and player_id = @player_id;

-- name: DeleteRosterEntry :execrows
delete from roster_entries
where league_id = @league_id and franchise_id = @franchise_id and player_id = @player_id;

-- name: SetRosterList :exec
update roster_entries set list = @list, reserved_at = @reserved_at
where league_id = @league_id and franchise_id = @franchise_id and player_id = @player_id;

-- name: InsertTransaction :exec
insert into transactions (dynasty_id, league_id, franchise_id, kind, player_id, draft_pick_id, trade_id, detail)
values (@dynasty_id, @league_id, @franchise_id, @kind, @player_id, @draft_pick_id, @trade_id, @detail);

-- name: ListActivity :many
select x.id, x.kind, x.detail, x.created_at,
       coalesce(l.competition, '')::text as competition,
       coalesce(f.name, '')::text        as franchise_name,
       coalesce(f.slug, '')::text        as franchise_slug,
       coalesce(p.full_name, '')::text   as player_name,
       coalesce(d.name, '')::text        as draft_name,
       coalesce(k.round, 0)::int         as pick_round
from transactions x
left join leagues l     on l.id = x.league_id
left join franchises f  on f.id = x.franchise_id
left join players p     on p.id = x.player_id
left join draft_picks k on k.id = x.draft_pick_id
left join drafts d      on d.id = k.draft_id
where x.dynasty_id = @dynasty_id
order by x.id desc
limit @page_size;
