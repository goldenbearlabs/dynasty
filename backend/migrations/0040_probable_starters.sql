-- +goose Up
-- Who a feed says will start in goal or on the mound, by day. A day's rows
-- are replaced whole each time the feed is read.
create table probable_starters (
 competition text not null,
 day date not null,
 player_id uuid not null references players(id) on delete cascade,
 confirmed boolean not null, -- false while only expected
 primary key (player_id, day)
);
create index probable_starters_day_idx on probable_starters (competition, day);

-- +goose Down
drop table probable_starters;
