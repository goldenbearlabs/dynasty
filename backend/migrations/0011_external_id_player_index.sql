-- +goose Up

-- Looking up a player's ids by player, as merge suggestions do, had no index.
create index player_external_ids_player_idx on player_external_ids (player_id);

-- +goose Down
drop index player_external_ids_player_idx;
