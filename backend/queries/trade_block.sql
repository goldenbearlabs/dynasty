-- name: ListTradeBlock :many
-- Every player on a trade block in a dynasty, still on the roster that put him there.
select b.league_id, l.competition, b.franchise_id, b.player_id, b.note, b.added_at,
       p.full_name, p.positions, p.headshot_url, coalesce(t.abbrev, '')::text as team_abbrev, r.list
from trade_block b
join roster_entries r on r.league_id = b.league_id and r.player_id = b.player_id and r.franchise_id = b.franchise_id
join leagues l on l.id = b.league_id
join players p on p.id = b.player_id
left join pro_teams t on t.id = p.pro_team_id
where l.dynasty_id = @dynasty_id
order by b.added_at desc;

-- name: PutOnTradeBlock :exec
insert into trade_block (league_id, player_id, franchise_id, note)
values (@league_id, @player_id, @franchise_id, @note)
on conflict (league_id, player_id) do update set franchise_id = excluded.franchise_id, note = excluded.note,
  added_at = case when trade_block.franchise_id = excluded.franchise_id then trade_block.added_at else now() end;

-- name: TakeOffTradeBlock :exec
delete from trade_block where league_id = @league_id and player_id = @player_id and franchise_id = @franchise_id;

-- name: ListTradeWants :many
select w.franchise_id, w.league_id, l.competition, w.positions, w.note, w.updated_at
from trade_wants w
join leagues l on l.id = w.league_id
where l.dynasty_id = @dynasty_id
order by w.updated_at desc;

-- name: SetTradeWants :exec
insert into trade_wants (franchise_id, league_id, positions, note)
values (@franchise_id, @league_id, @positions, @note)
on conflict (franchise_id, league_id) do update set positions = excluded.positions, note = excluded.note, updated_at = now();

-- name: ClearTradeWants :exec
delete from trade_wants where franchise_id = @franchise_id and league_id = @league_id;

-- name: ListTradeTargets :many
-- Targets that concern one franchise: those it has set, and those others
-- have set on its players. A target counts while someone else holds him.
select tt.franchise_id as by_franchise_id, r.franchise_id as owner_franchise_id, tt.league_id, l.competition,
       tt.player_id, tt.created_at, p.full_name, p.positions, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev, r.list
from trade_targets tt
join roster_entries r on r.league_id = tt.league_id and r.player_id = tt.player_id and r.franchise_id <> tt.franchise_id
join leagues l on l.id = tt.league_id
join players p on p.id = tt.player_id
left join pro_teams t on t.id = p.pro_team_id
where tt.franchise_id = @franchise_id or r.franchise_id = @franchise_id
order by tt.created_at desc;

-- name: SetTradeTarget :exec
insert into trade_targets (franchise_id, league_id, player_id) values (@franchise_id, @league_id, @player_id)
on conflict do nothing;

-- name: RemoveTradeTarget :exec
delete from trade_targets where franchise_id = @franchise_id and league_id = @league_id and player_id = @player_id;
