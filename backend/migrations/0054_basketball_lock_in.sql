-- +goose Up
-- Versioned by scoring-window start, never by deployment time. Matchups
-- beginning before 2026-11-16 retain their original selections and scoring.
create table basketball_lineup_windows (
  league_id uuid not null references leagues(id) on delete cascade,
  franchise_id uuid not null references franchises(id) on delete cascade,
  effective_on date not null,
  primary key (league_id, franchise_id, effective_on)
);

-- The initial snapshot and every subsequent move, including bench moves.
-- Unlike date-based lineups, these distinguish moves before and after tipoff.
create table basketball_lineup_moves (
  league_id uuid not null,
  franchise_id uuid not null,
  effective_on date not null,
  player_id uuid not null references players(id) on delete cascade,
  changed_at timestamptz not null,
  slot text not null, -- empty means bench
  primary key (league_id, franchise_id, effective_on, player_id, changed_at),
  foreign key (league_id, franchise_id, effective_on)
    references basketball_lineup_windows on delete cascade
);

create table basketball_game_locks (
  league_id uuid not null,
  franchise_id uuid not null,
  effective_on date not null,
  player_id uuid not null references players(id) on delete cascade,
  game_id uuid not null references games(id) on delete cascade,
  slot text not null,
  locked_at timestamptz not null default now(),
  primary key (league_id, franchise_id, effective_on, player_id, game_id),
  foreign key (league_id, franchise_id, effective_on)
    references basketball_lineup_windows on delete cascade
);

-- +goose StatementBegin
create function basketball_slot_at(lid uuid, fid uuid, win date, pid uuid, moment timestamptz)
returns text language sql stable as $$
  select coalesce(case when exists (
    select 1 from basketball_lineup_windows w
    where w.league_id = lid and w.franchise_id = fid and w.effective_on = win
  ) then (
    select m.slot from basketball_lineup_moves m
    where m.league_id = lid and m.franchise_id = fid and m.effective_on = win
      and m.player_id = pid and m.changed_at <= moment
    order by m.changed_at desc limit 1
  ) else (
    select e.slot from lineup_entries e
    where e.league_id = lid and e.franchise_id = fid and e.player_id = pid
      and e.effective_on = (select max(n.effective_on) from lineups n
        where n.league_id = lid and n.franchise_id = fid and n.effective_on <= win)
    limit 1
  ) end, '')
$$;
-- +goose StatementEnd

-- Shared by the editor and official scoring so eligibility and fallback agree.
-- A locked game keeps its identity through later stat corrections.
-- +goose StatementBegin
create function basketball_game_choices(lid uuid, fid uuid, win date, win_end date, moment timestamptz)
returns table (player_id uuid, game_id uuid, slot text, eligible boolean,
               locked_in boolean, counts boolean, can_lock_in boolean, lock_deadline timestamptz)
language sql stable as $$
  with rostered as (
    select re.player_id from roster_entries re
    where re.league_id = lid and re.franchise_id = fid and re.list = 'main'
    union
    select m.player_id from basketball_lineup_moves m
    where m.league_id = lid and m.franchise_id = fid and m.effective_on = win
    union
    select e.player_id from lineup_entries e
    where e.league_id = lid and e.franchise_id = fid
      and e.effective_on = (select max(n.effective_on) from lineups n
        where n.league_id = lid and n.franchise_id = fid and n.effective_on <= win)
  ), candidates as (
    select p.id as pid, g.id as gid, g.starts_at, g.status,
      basketball_slot_at(lid, fid, win, p.id, g.starts_at) as played_slot,
      basketball_slot_at(lid, fid, win, p.id, 'infinity'::timestamptz) as current_slot,
      coalesce((sl.stats->>'min')::numeric, 0) > 0 as played,
      gl.game_id is not null as locked,
      coalesce(gl.slot, '') as locked_slot,
      least(((win_end + 1)::timestamp + interval '5 hours') at time zone 'America/New_York',
        (select min(ng.starts_at) from games ng where ng.competition = l.competition
          and p.pro_team_id in (ng.home_team_id, ng.away_team_id) and ng.starts_at > g.starts_at)) as deadline,
      (select count(*) from basketball_game_locks x where x.league_id = lid
        and x.franchise_id = fid and x.effective_on = win and x.player_id = p.id) as used,
      case when exists (select 1 from periods per join seasons se on se.id = per.season_id
        where se.league_id = lid and per.double_games and per.starts_on = win and per.ends_on = win_end)
        then 2 else 1 end as game_limit,
      not exists (select 1 from games ng where ng.competition = l.competition
        and p.pro_team_id in (ng.home_team_id, ng.away_team_id)
        and ng.day between win and win_end and ng.status <> 'final') as finished
    from leagues l
    join players p on p.id in (select rostered.player_id from rostered)
    join games g on g.competition = l.competition and g.day between win and win_end
      and (p.pro_team_id in (g.home_team_id, g.away_team_id)
        or exists (select 1 from stat_lines played where played.game_id = g.id and played.player_id = p.id)
        or exists (select 1 from basketball_game_locks kept where kept.league_id = lid
          and kept.franchise_id = fid and kept.effective_on = win and kept.player_id = p.id and kept.game_id = g.id))
    left join stat_lines sl on sl.player_id = p.id and sl.game_id = g.id
    left join basketball_game_locks gl on gl.league_id = lid and gl.franchise_id = fid
      and gl.effective_on = win and gl.player_id = p.id and gl.game_id = g.id
    where l.id = lid
  ), ranked as (
    select c.*, row_number() over (partition by pid,
      (played_slot <> '' and played and status = 'final' and not locked)
      order by starts_at desc, gid desc) as fallback_number
    from candidates c
  )
  select pid, gid, case when locked then locked_slot else played_slot end,
    played_slot <> '', locked,
    locked or (played_slot <> '' and current_slot <> '' and played and status = 'final' and not locked
      and (finished or moment >= ((win_end + 1)::timestamp + interval '5 hours') at time zone 'America/New_York')
      and fallback_number <= game_limit - used),
    played_slot <> '' and played and status = 'final' and not locked and used < game_limit
      and moment < deadline,
    deadline
  from ranked
$$;
-- +goose StatementEnd

create trigger invalidate_response_cache after insert or update or delete or truncate on basketball_lineup_windows
  for each statement execute function bump_cache_revision('public');
create trigger invalidate_response_cache after insert or update or delete or truncate on basketball_lineup_moves
  for each statement execute function bump_cache_revision('public');
create trigger invalidate_response_cache after insert or update or delete or truncate on basketball_game_locks
  for each statement execute function bump_cache_revision('public');
create trigger unsettle_weeks after insert or update or delete on basketball_lineup_moves
  for each row execute function unsettle_weeks();
create trigger unsettle_periods after insert or update or delete on basketball_lineup_moves
  for each row execute function unsettle_periods();
create trigger unsettle_weeks after insert or update or delete on basketball_game_locks
  for each row execute function unsettle_weeks();
create trigger unsettle_periods after insert or update or delete on basketball_game_locks
  for each row execute function unsettle_periods();

-- +goose Down
drop function basketball_game_choices(uuid, uuid, date, date, timestamptz);
drop function basketball_slot_at(uuid, uuid, date, uuid, timestamptz);
drop table basketball_game_locks;
drop table basketball_lineup_moves;
drop table basketball_lineup_windows;
