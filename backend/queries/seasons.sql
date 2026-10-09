-- name: UpsertPlayerSeason :exec
insert into player_seasons (player_id, competition, year, label, team, league, games, stats, conference)
values (@player_id, @competition, @year, @label, @team, @league, @games, @stats, @conference)
on conflict (player_id, competition, year, team, league) do update
  set label = excluded.label, games = excluded.games, stats = excluded.stats, conference = excluded.conference, synced_at = now();

-- name: DeleteStalePlayerSeasons :exec
-- Removes a season's own-league lines that the latest sync did not write
-- again: a traded player's line under his old team, for instance.
delete from player_seasons
where competition = @competition and year = @year and league = '' and synced_at < @synced_before;

-- name: ListSeasonYears :many
-- The seasons already stored for a competition's own league.
select distinct year from player_seasons where competition = @competition and league = '';

-- name: ListPlayerSeasons :many
-- A player's seasons, newest first. Within a year his own league comes
-- before any other.
select * from player_seasons
where player_id = @player_id
order by year desc, league <> '', competition, team;

-- name: MarkCareerChecked :exec
update players set career_checked_at = now() where id = @id;

-- name: MovePlayerSeasons :exec
-- Keeps the surviving player's own line where both have one.
update player_seasons ps set player_id = @keep_id
where ps.player_id = @duplicate_id
  and not exists (
    select 1 from player_seasons k
    where k.player_id = @keep_id and k.competition = ps.competition and k.year = ps.year
      and k.team = ps.team and k.league = ps.league);

-- name: ListSeasonsMissingConferences :many
-- Metadata added after the original imports must be backfilled even when
-- these seasons are older than the ordinary history window. Partially known
-- seasons may contain non-Division-I teams, which intentionally stay unknown.
select year from player_seasons
where competition = @competition and competition = 'cbb' and league = ''
group by year having bool_and(conference = '');
