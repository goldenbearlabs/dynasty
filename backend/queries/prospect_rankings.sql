-- name: DeleteProspectRankingSource :exec
delete from prospect_rankings where source= @source;

-- name: InsertProspectRanking :exec
insert into prospect_rankings(player_id,source,year,rank,eta,source_url)
values(@player_id,@source,@year,@rank,@eta,@source_url);

-- name: MoveProspectRankings :exec
with moved as (
 insert into prospect_rankings(player_id,source,year,rank,eta,source_url,synced_at)
 select @keep_id,r.source,r.year,r.rank,r.eta,r.source_url,r.synced_at from prospect_rankings r where r.player_id = @duplicate_id
 on conflict (source,player_id) do nothing
)
delete from prospect_rankings r where r.player_id = @duplicate_id;

-- name: LockProspectRankingSource :exec
select pg_advisory_xact_lock(hashtextextended('prospect_rankings:' || @source::text,0));

-- name: ProspectRankingYear :one
select coalesce(max(year),0)::int from prospect_rankings where source = @source;

-- name: ListProspectRankings :many
-- Every ranked prospect, for a tag beside his name wherever he appears.
select player_id, rank, year, eta, source_url from prospect_rankings where source = 'mlb_pipeline' order by rank;
