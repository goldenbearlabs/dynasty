-- +goose Up
-- Future drafts may predate most of the dynasty's managers. Fill only
-- unannounced, unstarted drafts; announced boards remain commissioner-owned.
-- +goose StatementBegin
create function reconcile_placeholder_draft_picks(target_dynasty uuid) returns void
language plpgsql as $$
declare
  future_draft record;
  rounds integer;
  last_position integer;
begin
  for future_draft in
    select id from drafts
    where dynasty_id = target_dynasty and kind = 'seasonal'
      and status = 'scheduled' and is_placeholder
    order by id for update
  loop
    select coalesce(max(round), 0), coalesce(max(position), 0)
      into rounds, last_position from draft_picks where draft_id = future_draft.id;

    -- Original ownership identifies an existing entitlement even after a trade.
    insert into draft_picks (draft_id, round, position, original_franchise_id, current_franchise_id)
    select future_draft.id, missing.round,
           last_position + row_number() over (order by missing.round, f.name, f.id),
           f.id, f.id
    from franchises f cross join generate_series(1, rounds) as missing(round)
    where f.dynasty_id = target_dynasty and not exists (
      select 1 from draft_picks k where k.draft_id = future_draft.id
        and k.round = missing.round and k.original_franchise_id = f.id
    );

    -- Keep existing relative order within a round and append missing managers.
    -- The position constraint is deferred, so all slots move atomically.
    with ordered as (
      select id, row_number() over (order by round, position)::integer as position
      from draft_picks where draft_id = future_draft.id
    )
    update draft_picks k set position = o.position
    from ordered o where k.id = o.id and k.position <> o.position;
  end loop;
end;
$$;
-- +goose StatementEnd

-- Adding a franchise and its future picks is one transaction.
-- +goose StatementBegin
create function franchise_future_draft_picks() returns trigger
language plpgsql as $$
begin
  perform reconcile_placeholder_draft_picks(new.dynasty_id);
  return new;
end;
$$;
-- +goose StatementEnd
create trigger franchise_future_draft_picks after insert on franchises
for each row execute function franchise_future_draft_picks();

select reconcile_placeholder_draft_picks(id) from dynasties;

-- +goose Down
drop trigger franchise_future_draft_picks on franchises;
drop function franchise_future_draft_picks();
drop function reconcile_placeholder_draft_picks(uuid);
