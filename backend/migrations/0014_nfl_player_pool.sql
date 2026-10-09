-- +goose Up

-- Remove NFL players imported before the offensive-only pool was enforced.
-- Keep any player referenced by fantasy history, as roster sync does.
-- Season totals, external IDs and other player-owned data cascade with them.
delete from players p
where p.competition = 'nfl'
  and cardinality(p.positions) > 0
  and not p.positions && array['QB', 'RB', 'FB', 'WR', 'TE']::text[]
  and not exists (select 1 from roster_entries r where r.player_id = p.id)
  and not exists (select 1 from transactions x where x.player_id = p.id)
  and not exists (select 1 from draft_picks k where k.player_id = p.id)
  and not exists (select 1 from trade_items i where i.player_id = p.id);

-- +goose Down
-- Imported player data is restored by ingestion rather than rollback.
