-- name: PutOnWaivers :exec
insert into waivers (league_id, player_id, clears_at)
values (@league_id, @player_id, @clears_at)
on conflict (league_id, player_id) do update set clears_at = excluded.clears_at;

-- name: GetWaiver :one
select * from waivers where league_id = @league_id and player_id = @player_id;

-- name: ListWaivers :many
select w.player_id, w.clears_at, p.full_name, p.positions, p.status, p.note, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev
from waivers w
join players p on p.id = w.player_id
left join pro_teams t on t.id = p.pro_team_id
where w.league_id = @league_id
order by w.clears_at, p.full_name;

-- name: ListDueWaivers :many
select * from waivers where clears_at <= now() order by clears_at;

-- name: DeleteWaiver :exec
delete from waivers where league_id = @league_id and player_id = @player_id;

-- name: PutWaiverClaim :exec
-- A franchise has one live claim per player; claiming again replaces it.
insert into waiver_claims (league_id, franchise_id, player_id, drop_player_id, bid)
values (@league_id, @franchise_id, @player_id, @drop_player_id, @bid)
on conflict (league_id, franchise_id, player_id) where status = 'pending'
do update set drop_player_id = excluded.drop_player_id, bid = excluded.bid, created_at = now();

-- name: ListPendingClaims :many
select * from waiver_claims
where league_id = @league_id and player_id = @player_id and status = 'pending'
order by created_at;

-- name: ListFranchiseClaims :many
-- A franchise's live claims, then the ones settled most recently.
select c.id, c.player_id, c.bid, c.status, c.reason, c.created_at, c.resolved_at,
       p.full_name as player_name,
       coalesce(d.full_name, '')::text as drop_player_name
from waiver_claims c
join players p on p.id = c.player_id
left join players d on d.id = c.drop_player_id
where c.league_id = @league_id and c.franchise_id = @franchise_id and c.status <> 'cancelled'
order by (c.status = 'pending') desc, coalesce(c.resolved_at, c.created_at) desc
limit 30;

-- name: CancelWaiverClaim :execrows
update waiver_claims set status = 'cancelled', resolved_at = now()
where id = @id and franchise_id = @franchise_id and status = 'pending';

-- name: ResolveWaiverClaim :exec
update waiver_claims set status = @status, reason = @reason, resolved_at = now() where id = @id;

-- name: ListWaiverStandings :many
-- Every franchise in waiver order for one league: whoever has gone longest
-- without winning a claim is first. spent is what its winning bids have
-- cost since the league's current season began.
select f.id as franchise_id, f.name, f.slug,
       coalesce(sum(c.bid) filter (where c.status = 'won' and c.resolved_at >= coalesce(
         (select max(s.starts_on) from seasons s where s.league_id = @league_id and s.starts_on <= current_date)::timestamptz,
         '-infinity')), 0)::int as spent
from franchises f
left join waiver_claims c on c.franchise_id = f.id and c.league_id = @league_id
where f.dynasty_id = @dynasty_id
group by f.id
order by max(c.resolved_at) filter (where c.status = 'won') asc nulls first, f.name;

-- name: CountAcquisitions :one
-- Free agent adds and waiver claims a franchise has made itself since a
-- moment; a commissioner's placements do not count.
select count(*) from transactions
where league_id = @league_id and franchise_id = @franchise_id
  and kind in ('add', 'claim') and created_at >= @since
  and coalesce((detail->>'forced')::boolean, false) = false;
