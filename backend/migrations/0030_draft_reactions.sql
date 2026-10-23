-- +goose Up
-- Managers' emoji reactions to picks that have been made, one of each emoji per franchise.
create table draft_pick_reactions (
 pick_id uuid not null references draft_picks(id) on delete cascade,
 franchise_id uuid not null references franchises(id) on delete cascade,
 emoji text not null,
 created_at timestamptz not null default now(),
 primary key (pick_id, franchise_id, emoji)
);

-- +goose Down
drop table draft_pick_reactions;
