-- Stores hashed, revocable mobile tokens separately from browser cookie sessions.

-- +goose Up
create table mobile_sessions (
  token_hash bytea primary key,
  user_id uuid not null references users(id) on delete cascade,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);
create index mobile_sessions_user_idx on mobile_sessions (user_id);

-- +goose Down
drop table mobile_sessions;
