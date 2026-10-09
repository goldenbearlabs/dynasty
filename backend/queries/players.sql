-- name: UpsertProTeam :one
insert into pro_teams (competition, provider_id, abbrev, name, logo_url)
values (@competition, @provider_id, @abbrev, @name, @logo_url)
on conflict (competition, provider_id) do update
  set abbrev = excluded.abbrev, name = excluded.name, logo_url = excluded.logo_url
returning id;

-- name: FindPlayerByExternalID :one
select player_id from player_external_ids
where provider = @provider and provider_id = @provider_id;

-- name: InsertPlayer :one
with new_player as (
  insert into players (competition, status, full_name, positions, birth_date, pro_team_id, class, headshot_url)
  values (@competition, 'active', @full_name, @positions, @birth_date, @pro_team_id, @class, @headshot_url)
  returning id
)
insert into player_external_ids (provider, provider_id, player_id)
select @provider, @provider_id, id from new_player
returning player_id;

-- name: UpdatePlayer :one
-- Returns the competition the player was in before, so a move (a college
-- player reaching the NBA) can be acted on.
update players set
  eligible_since = case when competition <> @competition then now() else eligible_since end,
  competition    = @competition,
  status         = 'active',
  full_name      = @full_name,
  positions      = @positions,
  birth_date     = coalesce(@birth_date, birth_date),
  pro_team_id    = @pro_team_id,
  class          = @class,
  headshot_url   = @headshot_url,
  updated_at     = now()
where players.id = @id
returning (select was.competition from players was where was.id = players.id)::text as previous_competition;

-- name: MarkMissingInactive :execrows
update players set status = 'inactive', pro_team_id = null
where competition = @competition and status = 'active' and updated_at < @seen_before;

-- name: DeletePlayersOutsidePool :execrows
-- Removes players a narrowed competition no longer covers: at none of its
-- positions, or on a team it does not list. A player with any fantasy
-- history stays, and is marked inactive by the roster sync like anyone
-- else who has left.
delete from players p
where p.competition = @competition
  and ((cardinality(@positions::text[]) > 0 and cardinality(p.positions) > 0 and not p.positions && @positions::text[])
    or (@listed_teams_only::boolean and p.pro_team_id in (
          select t.id from pro_teams t
          where t.competition = @competition and t.provider_id <> all(@team_ids::text[]))))
  and not exists (select 1 from roster_entries r where r.player_id = p.id)
  and not exists (select 1 from transactions x where x.player_id = p.id)
  and not exists (select 1 from draft_picks k where k.player_id = p.id)
  and not exists (select 1 from trade_items i where i.player_id = p.id);

-- name: DetachUnlistedTeams :exec
-- Before DeleteUnlistedTeams: nothing may still point at a team being removed.
with gone as (
  select t.id from pro_teams t where t.competition = @competition and t.provider_id <> all(@team_ids::text[])
), released as (
  update players set pro_team_id = null where pro_team_id in (select id from gone)
)
update games set
  home_team_id = case when home_team_id in (select id from gone) then null else home_team_id end,
  away_team_id = case when away_team_id in (select id from gone) then null else away_team_id end
where home_team_id in (select id from gone) or away_team_id in (select id from gone);

-- name: DeleteUnlistedTeams :execrows
delete from pro_teams where competition = @competition and provider_id <> all(@team_ids::text[]);

-- name: ListPlayers :many
-- available_in narrows to players a league could acquire: they play in its
-- competition and are on nobody's roster there. draft_id does the same for
-- every league a draft covers.
select p.id, p.competition, p.status, p.full_name, p.positions, p.birth_date,
       p.class, p.note, p.headshot_url, p.eligible_since,
       coalesce(t.abbrev, '')::text as team_abbrev,
       coalesce(t.name, '')::text   as team_name,
       coalesce(f.name, '')::text   as owner_name,
       coalesce(f.slug, '')::text   as owner_slug,
       w.clears_at                  as waiver_until
from players p
left join pro_teams t      on t.id = p.pro_team_id
left join roster_entries r on r.player_id = p.id
left join franchises f     on f.id = r.franchise_id
left join waivers w        on w.player_id = p.id
where (@competition::text = '' or p.competition = @competition)
  and (@status::text = '' or p.status = @status)
  and (@search::text = '' or p.full_name ilike '%' || @search || '%')
  and (sqlc.narg('available_in')::uuid is null or (
        r.player_id is null
        and p.competition = (select l.competition from leagues l where l.id = sqlc.narg('available_in'))))
  and (sqlc.narg('draft_id')::uuid is null or (
        r.player_id is null
        and p.competition in (select l.competition from draft_leagues dl
                              join leagues l on l.id = dl.league_id
                              where dl.draft_id = sqlc.narg('draft_id'))))
order by p.full_name, p.id
limit @page_size offset @page_offset;

-- name: CountPlayers :one
select count(*)
from players p
left join roster_entries r on r.player_id = p.id
where (@competition::text = '' or p.competition = @competition)
  and (@status::text = '' or p.status = @status)
  and (@search::text = '' or p.full_name ilike '%' || @search || '%')
  and (sqlc.narg('available_in')::uuid is null or (
        r.player_id is null
        and p.competition = (select l.competition from leagues l where l.id = sqlc.narg('available_in'))))
  and (sqlc.narg('draft_id')::uuid is null or (
        r.player_id is null
        and p.competition in (select l.competition from draft_leagues dl
                              join leagues l on l.id = dl.league_id
                              where dl.draft_id = sqlc.narg('draft_id'))));

-- name: CountPlayersByCompetition :many
select competition, count(*) as players from players group by competition;

-- name: GetPlayer :one
select * from players where id = @id;

-- name: InsertProspect :one
with new_player as (
  insert into players (competition, status, full_name, positions, birth_date, note, headshot_url)
  values (@competition, 'prospect', @full_name, @positions, @birth_date, @note, @headshot_url)
  returning id
)
insert into player_external_ids (provider, provider_id, player_id)
select @provider, @provider_id, id from new_player
returning player_id;

-- name: UpdateProspect :exec
-- Only touches rows that are still prospects: a prospect feed must never
-- overwrite a player who has since arrived.
update players set
  full_name    = @full_name,
  positions    = @positions,
  birth_date   = coalesce(@birth_date, birth_date),
  note         = @note,
  headshot_url = @headshot_url,
  updated_at   = now()
where id = @id and status = 'prospect';

-- name: InsertManualPlayer :one
insert into players (competition, status, full_name, positions, birth_date, note)
values (@competition, @status, @full_name, @positions, @birth_date, @note)
returning id;

-- name: ListMergeSuggestions :many
-- A prospect and an arrived player with the same name in the same
-- competition may be one person entered through two feeds. Two rows with
-- ids from the same feed are known to be different people, so those pairs
-- are left out.
select a.id as prospect_id, a.full_name, a.competition, a.note as prospect_note,
       b.id as player_id, b.status as player_status, b.class as player_class,
       coalesce(t.name, '')::text as player_team
from players a
join players b on b.competition = a.competition
              and lower(b.full_name) = lower(a.full_name)
              and b.status <> 'prospect'
left join pro_teams t on t.id = b.pro_team_id
where a.status = 'prospect'
  -- Written as a lookup per pair: joining the ids to each other first
  -- pairs every id with every other from its feed, millions of rows.
  and not exists (
    select 1 from player_external_ids xa
    where xa.player_id = a.id
      and xa.provider in (select xb.provider from player_external_ids xb where xb.player_id = b.id))
order by a.full_name
limit 200;

-- name: MoveExternalIDs :exec
update player_external_ids set player_id = @keep_id where player_id = @duplicate_id;

-- name: MoveRosterEntries :exec
update roster_entries set player_id = @keep_id where player_id = @duplicate_id;

-- name: MoveTransactions :exec
update transactions set player_id = @keep_id where player_id = @duplicate_id;

-- name: KeepEarliestEligibility :exec
update players k set eligible_since = least(k.eligible_since, d.eligible_since)
from players d
where k.id = @keep_id and d.id = @duplicate_id;

-- name: DeletePlayer :exec
delete from players where id = @id;

-- name: ListRosteredInactive :many
-- Rostered players who have dropped out of their competition's feed: left
-- college without turning pro, retired, released abroad.
select p.id as player_id, p.full_name, p.positions, p.competition, p.note,
       re.league_id, re.list, f.id as franchise_id, f.name as franchise_name, f.slug as franchise_slug
from roster_entries re
join players p on p.id = re.player_id
join franchises f on f.id = re.franchise_id
where f.dynasty_id = @dynasty_id and p.status = 'inactive'
order by p.full_name;

-- name: MovePlayerCompetition :exec
-- Puts a player in another competition's pool by hand, as a prospect there.
update players set competition = @competition, status = 'prospect', eligible_since = now(), pro_team_id = null
where id = @id;

-- name: ListPlayerRosterEntries :many
select re.* from roster_entries re where re.player_id = @player_id;

-- name: GetLeagueByCompetition :one
select * from leagues where dynasty_id = @dynasty_id and competition = @competition;

-- name: GetPlayerProfile :one
-- A player's identity and current ownership for inline draft research.
select p.id, p.competition, p.status, p.full_name, p.positions, p.birth_date,
       p.class, p.note, p.headshot_url,
       coalesce(t.abbrev, '')::text as team_abbrev,
       coalesce(t.name, '')::text as team_name,
       coalesce(f.name, '')::text as owner_name,
       coalesce(f.slug, '')::text as owner_slug
from players p
left join pro_teams t on t.id = p.pro_team_id
left join roster_entries re on re.player_id = p.id
left join franchises f on f.id = re.franchise_id
where p.id = @id;

-- name: ListPlayerHistory :many
-- Last ten recorded finals in the player's current competition. Scoring
-- is calculated with current league rules, just as on the game pages.
select g.id, g.day, coalesce(a.abbrev, '')::text as away_abbrev,
       coalesce(h.abbrev, '')::text as home_abbrev, sl.stats,
       coalesce((select sum(stat.value::numeric * rule.value::numeric)
                 from jsonb_each_text(sl.stats) stat
                 join lateral jsonb_each_text(l.settings->'scoring') rule on rule.key = stat.key), 0)::float8 as points
from stat_lines sl
join games g on g.id = sl.game_id
left join pro_teams a on a.id = g.away_team_id
left join pro_teams h on h.id = g.home_team_id
left join leagues l on l.competition = g.competition
where sl.player_id = @player_id and g.status = 'final' and g.competition = @competition
order by g.starts_at desc, g.id desc
limit 10;

-- name: GetPlayerExternalID :one
-- The player's id in one feed's id space, if that feed knows him.
select provider_id from player_external_ids
where player_id = @player_id and provider = @provider;

-- name: AddPlayerExternalID :exec
-- Gives a player another feed id, when a second feed turns out to be
-- describing someone we already have.
insert into player_external_ids (provider, provider_id, player_id)
values (@provider, @provider_id, @player_id)
on conflict (provider, provider_id) do nothing;
