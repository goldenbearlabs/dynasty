-- name: CreateTrade :one
insert into trades (dynasty_id, proposed_by, note) values (@dynasty_id, @proposed_by, @note) returning *;

-- name: AddTradeParty :exec
insert into trade_parties (trade_id, franchise_id, accepted_at) values (@trade_id, @franchise_id, @accepted_at);

-- name: AddTradeItem :exec
insert into trade_items (trade_id, from_franchise, to_franchise, league_id, player_id, draft_pick_id)
values (@trade_id, @from_franchise, @to_franchise, @league_id, @player_id, @draft_pick_id);

-- name: LockTrade :one
select * from trades where id = @id for update;

-- name: ListTrades :many
select * from trades where dynasty_id = @dynasty_id order by created_at desc limit 200;

-- name: ListTradeParties :many
select p.* from trade_parties p
join trades t on t.id = p.trade_id
where t.dynasty_id = @dynasty_id;

-- name: ListTradePartiesOf :many
select * from trade_parties where trade_id = @trade_id;

-- name: AcceptTrade :exec
update trade_parties set accepted_at = now() where trade_id = @trade_id and franchise_id = @franchise_id;

-- name: SetTradeStatus :exec
update trades set
  status      = @status,
  resolved_at = case when @status in ('proposed', 'accepted') then null else now() end
where id = @id;

-- name: ListTradeItems :many
-- Every item of every trade in the dynasty, described for display.
select i.*,
       coalesce(p.full_name, '')::text     as player_name,
       coalesce(p.positions, '{}')::text[] as player_positions,
       coalesce(p.headshot_url, '')::text  as player_headshot,
       coalesce(l.competition, '')::text   as competition,
       coalesce(d.name, '')::text          as draft_name,
       coalesce(k.round, 0)::int           as pick_round,
       k.original_franchise_id             as pick_original_franchise_id
from trade_items i
join trades t on t.id = i.trade_id
left join players p     on p.id = i.player_id
left join leagues l     on l.id = i.league_id
left join draft_picks k on k.id = i.draft_pick_id
left join drafts d      on d.id = k.draft_id
where t.dynasty_id = @dynasty_id
order by i.id;

-- name: ListTradeItemsOf :many
select * from trade_items where trade_id = @trade_id order by id;

-- name: LockDraftPick :one
-- A pick with the state of its draft, locked for a trade.
select k.*, d.status as draft_status
from draft_picks k
join drafts d on d.id = k.draft_id
where k.id = @id
for update of k;

-- name: SetRosterOwner :exec
update roster_entries set franchise_id = @to_franchise, acquired_via = 'trade', acquired_at = now()
where league_id = @league_id and player_id = @player_id;

-- name: SetDraftPickOwner :exec
update draft_picks set current_franchise_id = @franchise_id where id = @id;

-- name: ListFranchisePicks :many
-- Unused picks a franchise holds, soonest draft first.
select k.id, k.round, k.position, k.original_franchise_id,
       d.id as draft_id, d.name as draft_name, d.year, d.status as draft_status,
       (select coalesce(array_agg(l.competition), '{}')::text[]
        from draft_leagues dl join leagues l on l.id = dl.league_id
        where dl.draft_id = d.id) as competitions
from draft_picks k
join drafts d on d.id = k.draft_id
where k.current_franchise_id = @franchise_id and k.player_id is null and d.status <> 'complete'
order by d.year, d.name, k.position;

-- name: ListDraftPickLeagues :many
-- The leagues a pick's draft covers.
select l.* from leagues l
join draft_leagues dl on dl.league_id = l.id
join draft_picks k on k.draft_id = dl.draft_id
where k.id = @id;

-- name: SeasonalDraftExists :one
select exists (
  select 1 from drafts d
  join draft_leagues dl on dl.draft_id = d.id
  where dl.league_id = @league_id and d.kind = 'seasonal' and d.year = @year
);

-- name: MoveTradeItems :exec
update trade_items set player_id = @keep_id where player_id = @duplicate_id;
