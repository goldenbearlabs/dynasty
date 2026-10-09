-- name: GetSecret :one
select value from secrets where name = @name;

-- name: InsertSecret :exec
-- Keeps the first value written if two servers start at once.
insert into secrets (name, value) values (@name, @value) on conflict (name) do nothing;

-- name: CreateUser :one
insert into users (email, password_hash) values (@email, @password_hash) returning *;

-- name: GetUserByEmail :one
select * from users where email = @email;

-- name: GetUser :one
select * from users where id = @id;

-- name: SetUserCredentials :exec
-- Changing credentials signs out everything issued before now.
update users set email = @email, password_hash = @password_hash, sessions_valid_from = now()
where id = @id;

-- name: GetFranchiseByUser :one
select * from franchises where user_id = @user_id;

-- name: ClaimFranchise :exec
-- Links the account and uses up the invite link.
update franchises set user_id = @user_id, invite_token = null where id = @id;

-- name: ListInvites :many
select f.*, coalesce(u.email, '')::text as email
from franchises f
left join users u on u.id = f.user_id
where f.dynasty_id = @dynasty_id
order by f.name;
