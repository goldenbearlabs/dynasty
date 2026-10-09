-- +goose Up

-- ---- accounts ----

-- A person who signs in. sessions_valid_from lets every existing sign-in be
-- cancelled at once: tokens issued before it are refused.
create table users (
  id                  uuid primary key default gen_random_uuid(),
  email               text not null unique, -- stored lower-case
  password_hash       text not null,
  sessions_valid_from timestamptz not null default now(),
  created_at          timestamptz not null default now()
);

-- A franchise is run by one account. Its invite link is how that account is
-- created (or its password reset); the link is cleared once used.
alter table franchises add column user_id uuid unique references users(id);
alter table franchises alter column invite_token drop not null;

-- Server-held secrets, generated on first start so nothing needs configuring.
create table secrets (
  name  text primary key,
  value bytea not null
);

-- ---- trades ----

create table trades (
  id          uuid primary key default gen_random_uuid(),
  dynasty_id  uuid not null references dynasties(id),
  -- proposed: waiting on a party. accepted: everyone agreed, waiting on the
  -- commissioner. executed: assets moved. The rest are endings.
  status      text not null default 'proposed'
              check (status in ('proposed', 'accepted', 'executed', 'rejected', 'cancelled', 'reversed')),
  proposed_by uuid not null references franchises(id),
  note        text not null default '',
  created_at  timestamptz not null default now(),
  resolved_at timestamptz
);

-- A table of its own, so a trade can have more than two sides.
create table trade_parties (
  trade_id     uuid not null references trades(id) on delete cascade,
  franchise_id uuid not null references franchises(id),
  accepted_at  timestamptz,
  primary key (trade_id, franchise_id)
);

-- One asset changing hands. A player names the league he is rostered in, so
-- one trade can span sports; a pick carries its league through its draft.
create table trade_items (
  id             uuid primary key default gen_random_uuid(),
  trade_id       uuid not null references trades(id) on delete cascade,
  from_franchise uuid not null references franchises(id),
  to_franchise   uuid not null references franchises(id),
  league_id      uuid references leagues(id),
  player_id      uuid references players(id),
  draft_pick_id  uuid references draft_picks(id),
  check ((player_id is not null and league_id is not null and draft_pick_id is null)
      or (player_id is null and league_id is null and draft_pick_id is not null))
);
create index trade_items_trade_idx on trade_items (trade_id);

alter table transactions add column trade_id uuid references trades(id);

-- +goose Down
alter table transactions drop column trade_id;
drop table trade_items;
drop table trade_parties;
drop table trades;
drop table secrets;
alter table franchises drop column user_id;
drop table users;
