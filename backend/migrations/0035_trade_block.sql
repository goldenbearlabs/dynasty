-- +goose Up
-- The trade market: players a manager will listen to offers for, what each
-- manager is looking for in each league, and players a manager wants from
-- someone else. An entry counts only while the player is still on that
-- roster, so a trade or a drop retires it.
create table trade_block (
  league_id    uuid not null references leagues(id) on delete cascade,
  player_id    uuid not null references players(id) on delete cascade,
  franchise_id uuid not null references franchises(id) on delete cascade,
  note         text not null default '',
  added_at     timestamptz not null default now(),
  primary key (league_id, player_id)
);

create table trade_wants (
  franchise_id uuid not null references franchises(id) on delete cascade,
  league_id    uuid not null references leagues(id) on delete cascade,
  positions    text[] not null default '{}',
  note         text not null default '',
  updated_at   timestamptz not null default now(),
  primary key (franchise_id, league_id)
);

create table trade_targets (
  franchise_id uuid not null references franchises(id) on delete cascade, -- who wants him
  league_id    uuid not null references leagues(id) on delete cascade,
  player_id    uuid not null references players(id) on delete cascade,
  created_at   timestamptz not null default now(),
  primary key (franchise_id, league_id, player_id)
);

-- +goose Down
drop table trade_targets;
drop table trade_wants;
drop table trade_block;
