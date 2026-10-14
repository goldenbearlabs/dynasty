-- +goose Up
alter table players add column pool_eligible boolean not null default true;

-- College draft boards imported by the old NFL/WNBA feeds are not part
-- of the professional player pool. Keep identities and historical picks.
update players set pool_eligible = false
where competition in ('nfl', 'wnba') and status = 'prospect'
  and note ~ '^[0-9]{4} draft prospect'
  and exists (select 1 from player_external_ids x where x.player_id = players.id
              and x.provider in ('espn_football', 'espn_womens_basketball'));

-- MLB draft selections alone do not prove that a player left college.
-- Use saved signing data to clean existing records without a network call.
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

-- +goose Down
alter table players drop column pool_eligible;
