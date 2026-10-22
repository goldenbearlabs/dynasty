-- +goose Up
-- Who turned a trade down, so the others hear about it. Empty for a
-- commissioner's veto.
alter table trades add column declined_by uuid references franchises(id);

-- +goose StatementBegin
create function notify_trade_declined() returns trigger language plpgsql as $$
declare manager text; proposer text;
begin
  if new.status <> 'rejected' or old.status not in ('proposed', 'accepted') then return new; end if;
  select name into manager from franchises where id = new.declined_by;
  select name into proposer from franchises where id = new.proposed_by;
  insert into notifications(franchise_id, kind, title, body, href)
  select p.franchise_id, 'trade',
    case when new.declined_by is null then 'The commissioner vetoed '
           || case when p.franchise_id = new.proposed_by then 'your trade' else 'the trade from ' || proposer end
         when p.franchise_id = new.proposed_by then manager || ' declined your trade'
         else manager || ' declined the trade from ' || proposer end,
    case when new.declined_by is null then 'Nothing changes hands.' else 'The offer is off. Nothing changes hands.' end,
    '/trades?trade=' || new.id::text
  from trade_parties p
  where p.trade_id = new.id and p.franchise_id is distinct from new.declined_by;
  return new;
end $$;
-- +goose StatementEnd
create trigger trade_declined_notifications after update of status on trades
  for each row execute function notify_trade_declined();

-- +goose Down
drop trigger trade_declined_notifications on trades;
drop function notify_trade_declined();
alter table trades drop column declined_by;
