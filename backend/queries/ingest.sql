-- name: StartIngestRun :one
insert into ingest_runs (competition, job) values (@competition, @job)
returning id, started_at;

-- name: FinishIngestRun :exec
update ingest_runs set
  finished_at   = now(),
  status        = case when @error::text = '' then 'ok' else 'error' end,
  rows_upserted = @rows_upserted,
  error         = @error
where id = @id;

-- name: ListIngestRuns :many
select * from ingest_runs order by id desc limit @page_size;

-- name: SaveRawPayload :exec
insert into raw_payloads (url, body) values (@url, @body)
on conflict (url) do update set body = excluded.body, fetched_at = now();
