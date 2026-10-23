-- Reconciles the early main-branch schema on databases that used versions 0029-0031 for mobile accounts.

-- +goose Up
alter table players add column if not exists pool_eligible boolean not null default true;

-- Match the professional prospect cleanup in main's migration 0029.
update players set pool_eligible = false
where competition in ('nfl', 'wnba') and status = 'prospect'
  and note ~ '^[0-9]{4} draft prospect'
  and exists (select 1 from player_external_ids x where x.player_id = players.id
              and x.provider in ('espn_football', 'espn_womens_basketball'));

with picks as (
 select pick->'person'->>'id' as id,
        coalesce(nullif(pick->>'signingBonus', ''), '0')::numeric > 0 as signed
 from raw_payloads r
 cross join lateral jsonb_array_elements(r.body::jsonb->'drafts'->'rounds') round
 cross join lateral jsonb_array_elements(round->'picks') pick
 where r.url ~ '^https://statsapi.mlb.com/api/v1/draft/[0-9]{4}$'
)
update players p set pool_eligible = false
where p.competition = 'mlb' and p.status = 'prospect'
  and p.note ~ '^[0-9]{4} draft, round '
  and exists (
   select 1 from player_external_ids x
   where x.player_id = p.id and x.provider = 'mlbam'
     and exists (select 1 from picks where picks.id = x.provider_id)
     and not exists (select 1 from picks where picks.id = x.provider_id and picks.signed)
  );

create table if not exists draft_pick_reactions (
 pick_id uuid not null references draft_picks(id) on delete cascade,
 franchise_id uuid not null references franchises(id) on delete cascade,
 emoji text not null,
 created_at timestamptz not null default now(),
 primary key (pick_id, franchise_id, emoji)
);

alter table lineup_entries add column if not exists counts_last boolean not null default false;

-- +goose Down
-- The original main-branch migrations own these objects, so rollback must not remove them.
select 1;
