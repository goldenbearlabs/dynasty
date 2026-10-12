-- +goose Up
create table watchlist_entries (
 franchise_id uuid not null references franchises(id) on delete cascade,
 player_id uuid not null references players(id) on delete cascade,
 priority bigint not null default 0,
 category text not null default '',
 notes text not null default '',
 created_at timestamptz not null default now(),
 primary key (franchise_id, player_id)
);

-- +goose Down
drop table watchlist_entries;
