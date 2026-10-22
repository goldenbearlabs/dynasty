-- +goose Up
create table notifications (
  id bigserial primary key,
  franchise_id uuid not null references franchises(id) on delete cascade,
  kind text not null check (kind in ('roster', 'trade', 'target')),
  title text not null,
  body text not null default '',
  href text not null,
  competition text not null default '',
  player_id uuid references players(id) on delete set null,
  league_id uuid references leagues(id) on delete cascade,
  waiver_until timestamptz,
  created_at timestamptz not null default now(),
  read_at timestamptz
);
create index notifications_inbox on notifications(franchise_id, id desc);
create index notifications_unread on notifications(franchise_id, id desc) where read_at is null;

-- Generate alongside the underlying write, including background jobs and
-- commissioner moves. Rollback also rolls back all recipient notifications.
-- +goose StatementBegin
create function notify_roster_transaction() returns trigger language plpgsql as $$
declare
  manager text;
  player text;
  sport text;
  verb text;
  destination text;
  clears timestamptz;
begin
  if new.player_id is null or new.franchise_id is null then return new; end if;
  select name into manager from franchises where id = new.franchise_id;
  select full_name into player from players where id = new.player_id;
  select competition into sport from leagues where id = new.league_id;
  verb := case new.kind when 'add' then 'added' when 'claim' then 'claimed'
    when 'drop' then 'dropped' when 'unsigned' then 'released unsigned player'
    when 'move' then 'moved' when 'sign' then 'signed' when 'pick' then 'drafted'
    when 'trade' then 'acquired in a trade:' when 'graduated' then 'carried over'
    when 'released' then 'released' else null end;
  if verb is null then return new; end if;
  destination := case new.detail->>'list' when 'main' then 'Main roster'
    when 'reserve' then 'Reserve' when 'rights' then 'Unsigned rights' else '' end;
  select clears_at into clears from waivers where league_id = new.league_id and player_id = new.player_id;
  insert into notifications(franchise_id, kind, title, body, href, competition, player_id, league_id, waiver_until)
  select f.id, 'roster', manager || ' ' || verb || ' ' || player,
    case when clears is not null then 'Placed on waivers.'
         when new.kind in ('drop', 'unsigned') then 'Available as a free agent.'
         else destination end,
    case when clears is not null then '/waivers?competition=' || coalesce(sport, '')
         else '/player/' || new.player_id::text end,
    coalesce(sport, ''), new.player_id, new.league_id, clears
  from franchises f where f.dynasty_id = new.dynasty_id
    and (f.id <> new.franchise_id or coalesce((new.detail->>'forced')::boolean, false));
  return new;
end $$;
-- +goose StatementEnd
create trigger roster_notifications after insert on transactions for each row execute function notify_roster_transaction();

-- +goose StatementBegin
create function notify_trade_offer() returns trigger language plpgsql as $$
declare offer trades%rowtype; manager text;
begin
  select * into offer from trades where id = new.trade_id;
  if new.franchise_id = offer.proposed_by then return new; end if;
  select name into manager from franchises where id = offer.proposed_by;
  insert into notifications(franchise_id, kind, title, body, href)
  values (new.franchise_id, 'trade', manager || ' proposed a trade to you',
    'Review the offer and choose whether to accept or decline.', '/trades?trade=' || new.trade_id::text);
  return new;
end $$;
-- +goose StatementEnd
create trigger trade_offer_notifications after insert on trade_parties for each row execute function notify_trade_offer();

-- +goose StatementBegin
create function notify_trade_target() returns trigger language plpgsql as $$
declare owner uuid; manager text; player text; sport text;
begin
  select franchise_id into owner from roster_entries where league_id = new.league_id and player_id = new.player_id;
  if owner is null or owner = new.franchise_id then return new; end if;
  select name into manager from franchises where id = new.franchise_id;
  select full_name into player from players where id = new.player_id;
  select competition into sport from leagues where id = new.league_id;
  insert into notifications(franchise_id, kind, title, body, href, competition, player_id, league_id)
  values (owner, 'target', manager || ' is targeting ' || player,
    'One of your players was added to their trade targets.', '/trades?view=block', sport, new.player_id, new.league_id);
  return new;
end $$;
-- +goose StatementEnd
create trigger trade_target_notifications after insert on trade_targets for each row execute function notify_trade_target();

-- +goose Down
drop trigger trade_target_notifications on trade_targets;
drop function notify_trade_target();
drop trigger trade_offer_notifications on trade_parties;
drop function notify_trade_offer();
drop trigger roster_notifications on transactions;
drop function notify_roster_transaction();
drop table notifications;
