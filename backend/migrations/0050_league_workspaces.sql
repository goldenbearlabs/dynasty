-- Saves new private league setups outside the legacy single-league gameplay tables.

-- +goose Up
create table if not exists league_workspaces (
  id uuid primary key default gen_random_uuid(),
  owner_user_id uuid not null references users(id) on delete cascade,
  name text not null,
  franchise_id uuid not null default gen_random_uuid(),
  franchise_name text not null,
  manager_name text not null,
  dynasty_settings jsonb not null,
  leagues_snapshot jsonb not null,
  created_at timestamptz not null default now()
);
create index if not exists league_workspaces_owner_idx on league_workspaces (owner_user_id, created_at);

-- +goose Down
drop table league_workspaces;
