-- name: CreateDraft :one
insert into drafts (dynasty_id, name, kind, year, pick_clock_seconds)
values (@dynasty_id, @name, @kind, @year, @pick_clock_seconds)
returning *;

-- name: AddDraftLeague :exec
insert into draft_leagues (draft_id, league_id) values (@draft_id, @league_id);

-- name: GetDraft :one
select * from drafts where id = @id;

-- name: LockDraft :one
-- Serializes everything that changes one draft.
select * from drafts where id = @id for update;

-- name: ListDrafts :many
select d.*,
       (select coalesce(array_agg(l.competition), '{}')::text[]
        from draft_leagues dl join leagues l on l.id = dl.league_id
        where dl.draft_id = d.id) as competitions,
       (select count(*) from draft_picks k where k.draft_id = d.id) as picks,
       (select count(*) from draft_picks k where k.draft_id = d.id and k.player_id is not null) as picks_made
from drafts d
where d.dynasty_id = @dynasty_id
order by d.created_at desc;

-- name: ListDraftLeagues :many
select l.* from leagues l
join draft_leagues dl on dl.league_id = l.id
where dl.draft_id = @draft_id;

-- name: DeleteDraft :exec
delete from drafts where id = @id;

-- name: SetDraftStatus :exec
update drafts set
  status           = @status,
  clock_expires_at = @clock_expires_at,
  completed_at     = case when @status = 'complete' then now() else null end
where id = @id;

-- name: SetDraftClock :exec
update drafts set pick_clock_seconds = @pick_clock_seconds where id = @id;

-- name: MarkLeaguesDrafted :exec
-- Players who were in the pool before this moment become free agents.
update leagues set last_draft_at = now()
where id in (select league_id from draft_leagues where draft_id = @draft_id);

-- name: LeagueHasOpenDraft :one
select exists (
  select 1 from drafts d
  join draft_leagues dl on dl.draft_id = d.id
  where dl.league_id = @league_id and d.status in ('live', 'paused')
);

-- name: ListExpiredDrafts :many
select id from drafts where status = 'live' and clock_expires_at < now();

-- name: ListDraftPicks :many
select k.*,
       coalesce(p.full_name, '')::text    as player_name,
       coalesce(p.positions, '{}')::text[] as player_positions,
       coalesce(p.headshot_url, '')::text as player_headshot,
       coalesce(l.competition, '')::text  as competition
from draft_picks k
left join players p on p.id = k.player_id
left join leagues l on l.id = k.league_id
where k.draft_id = @draft_id
order by k.position;

-- name: GetDraftPick :one
select * from draft_picks where id = @id;

-- name: InsertDraftPick :exec
insert into draft_picks (draft_id, round, position, original_franchise_id, current_franchise_id)
values (@draft_id, @round, @position, @original_franchise_id, @current_franchise_id);

-- name: UpdateDraftPickSlot :exec
update draft_picks set
  round                 = @round,
  position              = @position,
  original_franchise_id = @original_franchise_id,
  current_franchise_id  = @current_franchise_id
where id = @id and draft_id = @draft_id;

-- name: DeleteDraftPick :exec
delete from draft_picks where id = @id and draft_id = @draft_id;

-- name: MakeDraftPick :exec
update draft_picks set
  player_id   = @player_id,
  league_id   = @league_id,
  picked_at   = now(),
  auto_picked = @auto_picked
where id = @id;

-- name: PassDraftPick :exec
update draft_picks set passed_at = now() where id = @id;

-- name: StartupDraftExists :one
-- Whether a league has ever been in a startup draft, finished or not.
select exists (
  select 1 from drafts d
  join draft_leagues dl on dl.draft_id = d.id
  where d.kind = 'startup' and dl.league_id = @league_id
);

-- name: SkipDraftPick :exec
update draft_picks set skipped_at = now() where id = @id;

-- name: ClearDraftPick :exec
update draft_picks set player_id = null, league_id = null, picked_at = null, auto_picked = false, skipped_at = null, passed_at = null
where id = @id;

-- name: LastMadeDraftPick :one
select * from draft_picks
where draft_id = @draft_id and player_id is not null
order by picked_at desc, position desc
limit 1;

-- name: ListDraftQueue :many
select q.player_id, q.rank, p.full_name, p.positions, p.competition, p.status, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev
from draft_queue q
join players p on p.id = q.player_id
left join pro_teams t on t.id = p.pro_team_id
where q.draft_id = @draft_id and q.franchise_id = @franchise_id
order by q.rank;

-- name: ClearDraftQueue :exec
delete from draft_queue where draft_id = @draft_id and franchise_id = @franchise_id;

-- name: InsertDraftQueue :exec
insert into draft_queue (draft_id, franchise_id, player_id, rank)
values (@draft_id, @franchise_id, @player_id, @rank);

-- name: RemoveFromDraftQueues :exec
-- A drafted player leaves everyone's queue.
delete from draft_queue where draft_id = @draft_id and player_id = @player_id;
