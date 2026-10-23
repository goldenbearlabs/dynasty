-- Lets accounts exist before joining a league and later own one membership per league.

-- +goose Up
alter table franchises drop constraint if exists franchises_user_id_key;
create unique index franchises_dynasty_user_unique on franchises (dynasty_id, user_id) where user_id is not null;

-- Existing invited managers are already trusted. Only new registrations get a row here.
create table account_verifications (
  user_id uuid primary key references users(id) on delete cascade,
  token_hash bytea unique,
  expires_at timestamptz,
  verified_at timestamptz
);

create table account_password_resets (
  user_id uuid primary key references users(id) on delete cascade,
  token_hash bytea not null unique,
  expires_at timestamptz not null,
  created_at timestamptz not null default now()
);

-- +goose Down
drop table account_password_resets;
drop table account_verifications;
drop index franchises_dynasty_user_unique;
alter table franchises add constraint franchises_user_id_key unique (user_id);
