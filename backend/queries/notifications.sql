-- name: ListNotifications :many
select n.*, w.clears_at as current_waiver_until
from notifications n
left join waivers w on w.league_id = n.league_id and w.player_id = n.player_id
where n.franchise_id = @franchise_id
  and (@kind::text = '' or n.kind = @kind)
  and (not @unread_only::boolean or n.read_at is null)
order by n.id desc limit @page_size offset @page_offset;

-- name: CountNotifications :one
select count(*) from notifications
where franchise_id = @franchise_id
  and (@kind::text = '' or kind = @kind)
  and (not @unread_only::boolean or read_at is null);

-- name: NotificationSummary :one
select count(*) filter (where read_at is null)::int as unread,
       coalesce(max(id), 0)::bigint as latest_id
from notifications where franchise_id = @franchise_id;

-- name: ReadNotification :execrows
update notifications set read_at = case when @is_read::boolean then coalesce(read_at, now()) else null end
where id = @id and franchise_id = @franchise_id;

-- name: ReadAllNotifications :exec
update notifications set read_at = now()
where franchise_id = @franchise_id and id <= @through_id and read_at is null;
