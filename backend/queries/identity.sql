-- name: UpdateOrganizationIdentity :exec
update franchises set name = @name, image_url = @image_url where id = @id;

-- name: SetTeamIdentity :exec
insert into franchise_teams (franchise_id, league_id, name, image_url)
values (@franchise_id, @league_id, @name, @image_url)
on conflict (franchise_id, league_id) do update set name = excluded.name, image_url = excluded.image_url;

-- name: ListTeamIdentities :many
select ft.*, l.competition from franchise_teams ft
join leagues l on l.id = ft.league_id
where l.dynasty_id = @dynasty_id order by ft.franchise_id, l.competition;

-- name: GetPlayerNickname :one
select coalesce((select nickname from player_nicknames where franchise_id = @franchise_id and player_id = @player_id), '')::text as nickname;

-- name: SetPlayerNickname :exec
insert into player_nicknames (franchise_id, player_id, nickname) values (@franchise_id, @player_id, @nickname)
on conflict (franchise_id, player_id) do update set nickname = excluded.nickname;

-- name: DeletePlayerNickname :exec
delete from player_nicknames where franchise_id = @franchise_id and player_id = @player_id;

-- name: MovePlayerNicknames :exec
update player_nicknames n set player_id = @keep_id
where n.player_id = @duplicate_id
 and not exists (select 1 from player_nicknames k where k.franchise_id = n.franchise_id and k.player_id = @keep_id);
