-- name: ListRankings :many
select r.id, r.league_id, r.draft_id, r.name, r.updated_at,
       coalesce(l.competition, '')::text as competition,
       coalesce(d.name, '')::text as draft_name,
       (select count(*) from ranking_players rp where rp.ranking_id = r.id) as players
from rankings r
left join leagues l on l.id = r.league_id
left join drafts d on d.id = r.draft_id
where r.franchise_id = @franchise_id
order by l.competition, r.name, r.id;

-- name: GetRanking :one
select * from rankings where id = @id and franchise_id = @franchise_id;

-- name: CreateRanking :one
insert into rankings (franchise_id, league_id, draft_id, name)
values (@franchise_id, @league_id, @draft_id, @name)
returning *;

-- name: UpdateRanking :exec
update rankings set name = @name, draft_id = @draft_id, updated_at = now() where id = @id;

-- name: DeleteRanking :execrows
delete from rankings where id = @id and franchise_id = @franchise_id;

-- name: ClearRankingPlayers :exec
delete from ranking_players where ranking_id = @ranking_id;

-- name: InsertRankingPlayer :execrows
-- League boards cover one sport; combined startup boards cover their draft’s sports.
insert into ranking_players (ranking_id, player_id, rank)
select @ranking_id, p.id, @rank
from players p
join rankings r on r.id = @ranking_id
left join leagues l on l.id = r.league_id
where p.id = @player_id and (
 p.competition = l.competition or (r.league_id is null and exists (
  select 1 from draft_leagues dl join leagues covered on covered.id = dl.league_id
  where dl.draft_id = r.draft_id and covered.competition = p.competition
 ))
);

-- name: ListRankingPlayers :many
-- The ranked players in order, with who holds each one in the ranking's league.
select rp.player_id, rp.rank, p.full_name, p.positions, p.competition, p.status, p.note, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev,
       coalesce(f.name, '')::text   as owner_name
from ranking_players rp
join rankings r on r.id = rp.ranking_id
join players p on p.id = rp.player_id
left join pro_teams t on t.id = p.pro_team_id
left join roster_entries re on re.player_id = p.id and (re.league_id = r.league_id or (r.league_id is null and re.league_id in (select league_id from draft_leagues where draft_id = r.draft_id)))
left join franchises f on f.id = re.franchise_id
where rp.ranking_id = @ranking_id
order by rp.rank;

-- name: DraftCoversLeague :one
select exists (select 1 from draft_leagues where draft_id = @draft_id and league_id = @league_id);
