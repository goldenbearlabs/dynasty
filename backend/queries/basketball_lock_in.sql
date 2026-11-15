-- name: EnsureBasketballLineupWindow :execrows
insert into basketball_lineup_windows (league_id, franchise_id, effective_on)
values (@league_id, @franchise_id, @effective_on) on conflict do nothing;

-- name: RecordBasketballLineupMove :exec
insert into basketball_lineup_moves (league_id, franchise_id, effective_on, player_id, changed_at, slot)
values (@league_id, @franchise_id, @effective_on, @player_id, @changed_at, @slot)
on conflict (league_id, franchise_id, effective_on, player_id, changed_at) do update set slot = excluded.slot;

-- name: ListBasketballGameChoices :many
select player_id::uuid, game_id::uuid, slot::text, eligible::boolean, locked_in::boolean,
       counts::boolean, can_lock_in::boolean, lock_deadline::timestamptz
from basketball_game_choices(@league_id::uuid, @franchise_id::uuid, @from_day::date, @to_day::date, @as_of::timestamptz);

-- name: LockBasketballGame :exec
insert into basketball_game_locks (league_id, franchise_id, effective_on, player_id, game_id, slot)
values (@league_id, @franchise_id, @effective_on, @player_id, @game_id, @slot);

-- name: SeedBasketballLineupWindow :exec
-- Include starters who have just left the main roster: roster exits may
-- update ownership before they remove the old lineup entry.
insert into basketball_lineup_moves (league_id, franchise_id, effective_on, player_id, changed_at, slot)
select e.league_id, e.franchise_id, @effective_on::date, e.player_id,
       @changed_at::timestamptz, e.slot
from lineup_entries e
where e.league_id = @league_id and e.franchise_id = @franchise_id
  and e.effective_on = (select max(n.effective_on) from lineups n
    where n.league_id = @league_id and n.franchise_id = @franchise_id and n.effective_on <= @effective_on::date)
on conflict do nothing;

-- name: BenchFutureBasketballLineups :exec
insert into basketball_lineup_moves (league_id, franchise_id, effective_on, player_id, changed_at, slot)
select w.league_id, w.franchise_id, w.effective_on, @player_id::uuid,
       (w.effective_on::timestamp + interval '5 hours') at time zone 'America/New_York', ''
from basketball_lineup_windows w
where w.league_id = @league_id and w.franchise_id = @franchise_id and w.effective_on > @day::date
on conflict (league_id, franchise_id, effective_on, player_id, changed_at) do update set slot = '';
