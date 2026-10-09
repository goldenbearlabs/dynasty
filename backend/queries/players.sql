-- name: UpsertProTeam :one
insert into pro_teams (competition, provider_id, abbrev, name, logo_url, conference)
values (@competition, @provider_id, @abbrev, @name, @logo_url, @conference)
on conflict (competition, provider_id) do update
  set abbrev = excluded.abbrev, name = excluded.name, logo_url = excluded.logo_url, conference = excluded.conference
returning id;

-- name: FindPlayersByExternalIDs :many
-- Many feed ids at once; the two lists are read as pairs.
select provider, provider_id, player_id from player_external_ids
where (provider, provider_id) in (select unnest(@providers::text[]), unnest(@provider_ids::text[]));

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
--
-- Each player comes with one season's fantasy points under the rules of his
-- sport's league here: the sport's most recent season, or one further back
-- (season_back counts how many). season_index puts those points on a scale
-- shared by every sport, so players can be compared across leagues: 100 is
-- the average of the players a league that size would roster (the top
-- franchises x main-roster spots by points), and 15 is one standard
-- deviation among them. sort_by "points" or "index" puts the highest first.
with seasons as (
  select distinct competition, year from stat_seasons
), reference as materialized (
  -- the season with exactly season_back newer ones in its sport
  select s.competition, s.year from seasons s
  where (select count(*) from seasons newer where newer.competition = s.competition and newer.year > s.year) = @season_back::int
), scored as materialized (
  -- points are stored with each season line, under the rules of the league
  -- here that plays its sport
  select ps.player_id, ps.competition, max(ps.label)::text as label,
         sum(ps.points) as points, sum(ps.eligible_points) as eligible_points
  from player_seasons ps
  join reference on reference.competition = ps.competition and reference.year = ps.year
  where ps.league = '' and ps.points is not null and (@competition::text = '' or ps.competition = @competition)
  group by ps.player_id, ps.competition
), rostered as materialized (
  -- The players a league would hold, by points: what "average" is measured on.
  select placed.competition, avg(placed.points) as mean, stddev_pop(placed.points) as spread
  from (select sc.competition, sc.eligible_points as points,
               row_number() over (partition by sc.competition order by sc.eligible_points desc) as place
        from scored sc where sc.eligible_points is not null) placed
  join (select l.competition,
               greatest(1, (l.settings->'roster'->>'main')::int
                           * (select count(*) from franchises f where f.dynasty_id = l.dynasty_id)) as spots
        from leagues l) sizes on sizes.competition = placed.competition
  where placed.place <= sizes.spots
  group by placed.competition
)
select p.id, p.competition, p.status, p.full_name, p.positions, p.birth_date,
       p.class, p.note, p.headshot_url, p.eligible_since,
       coalesce(t.abbrev, '')::text as team_abbrev,
       coalesce(t.name, '')::text   as team_name,
       coalesce(f.name, '')::text   as owner_name,
       coalesce(f.slug, '')::text   as owner_slug,
       w.clears_at                  as waiver_until,
       coalesce(sc.label, '')::text     as season,
       coalesce(sc.points, 0)::float8   as season_points,
       coalesce(case when rostered.spread > 0 then 100 + 15 * (sc.eligible_points - rostered.mean) / rostered.spread end, 0)::float8 as season_index
from players p
left join pro_teams t      on t.id = p.pro_team_id
left join roster_entries r on r.player_id = p.id
left join franchises f     on f.id = r.franchise_id
left join waivers w        on w.player_id = p.id
left join scored sc        on sc.player_id = p.id and sc.competition = p.competition
left join rostered         on rostered.competition = p.competition
where (@competition::text = '' or p.competition = @competition)
  and (@status::text = '' or p.status = @status)
  and (@search::text = '' or p.full_name ilike '%' || @search || '%')
  and (sqlc.narg('available_in')::uuid is null or (
        r.player_id is null
        and p.competition = (select l.competition from leagues l where l.id = sqlc.narg('available_in'))))
  and (sqlc.narg('draft_id')::uuid is null or (
        r.player_id is null
        and exists (select 1 from draft_leagues dl
                    join leagues l on l.id = dl.league_id
                    where dl.draft_id = sqlc.narg('draft_id') and l.competition = p.competition)))
order by case @sort_by::text
           when 'points' then sc.points
           when 'index' then case when rostered.spread > 0 then (sc.eligible_points - rostered.mean) / rostered.spread end
         end desc nulls last,
         p.full_name, p.id
limit @page_size offset @page_offset;

-- name: ListStatSeasons :many
-- The seasons there are stats for in each sport, newest first.
select competition, year, max(label)::text as label
from stat_seasons
group by competition, year
order by competition, year desc;

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
        and exists (select 1 from draft_leagues dl
                    join leagues l on l.id = dl.league_id
                    where dl.draft_id = sqlc.narg('draft_id') and l.competition = p.competition)));

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
       coalesce(sl.points, 0)::float8 as points
from stat_lines sl
join games g on g.id = sl.game_id
left join pro_teams a on a.id = g.away_team_id
left join pro_teams h on h.id = g.home_team_id
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

-- name: GetPlayerConference :one
select coalesce(t.conference, '')::text as conference
from players p left join pro_teams t on t.id = p.pro_team_id
where p.id = @id;
