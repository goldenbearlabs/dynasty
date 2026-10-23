-- +goose Up
create table prospect_rankings (
  player_id uuid not null references players(id) on delete cascade,
  source text not null,
  year int not null check (year between 1900 and 2200),
  rank int not null check (rank between 1 and 100),
  eta text not null default '',
  source_url text not null,
  synced_at timestamptz not null default now(),
  primary key (source,player_id)
);
create index prospect_rankings_player_idx on prospect_rankings(player_id);
create trigger invalidate_response_cache after insert or update or delete or truncate on prospect_rankings
  for each statement execute function bump_cache_revision('research');

-- +goose Down
drop table prospect_rankings;
