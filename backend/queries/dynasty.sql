-- The screens assume a single dynasty per server; the schema does not.

-- name: GetDynasty :one
select * from dynasties order by created_at limit 1;

-- name: CreateDynasty :one
insert into dynasties (name, settings) values (@name, @settings) returning *;

-- name: UpdateDynasty :exec
update dynasties set name = @name, settings = @settings where id = @id;

-- name: CreateLeague :one
insert into leagues (dynasty_id, competition, name, settings)
values (@dynasty_id, @competition, @name, @settings)
returning *;

-- name: ListLeagues :many
select * from leagues where dynasty_id = @dynasty_id;

-- name: GetLeague :one
select * from leagues where id = @id;

-- name: UpdateLeagueSettings :exec
update leagues set settings = @settings where id = @id;

-- name: CreateFranchise :one
insert into franchises (dynasty_id, name, manager_name, slug, invite_token, is_commissioner)
values (@dynasty_id, @name, @manager_name, @slug, @invite_token, @is_commissioner)
returning *;

-- name: ListFranchises :many
select * from franchises where dynasty_id = @dynasty_id order by name;

-- name: GetFranchise :one
select * from franchises where id = @id;

-- name: GetFranchiseBySlug :one
select * from franchises where dynasty_id = @dynasty_id and slug = @slug;

-- name: GetFranchiseByInvite :one
select * from franchises where invite_token = @invite_token::text;

-- name: UpdateFranchise :exec
update franchises
set name = @name, manager_name = @manager_name, is_commissioner = @is_commissioner
where id = @id;

-- name: SetFranchiseToken :exec
update franchises set invite_token = @invite_token where id = @id;

-- name: ListLeagueCompetitions :many
-- The sports anyone here actually plays, across every dynasty.
select distinct competition from leagues;
