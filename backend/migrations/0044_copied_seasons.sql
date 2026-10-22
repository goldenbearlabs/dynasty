-- +goose Up
-- ESPN can answer an unpublished season request with the preceding season.
-- Remove only verified duplicate own-league lines stored under the wrong year;
-- preserve any unique or differing data for review rather than relabeling it.
delete from player_seasons bad
where bad.competition in ('nba', 'cbb') and bad.league = ''
  and bad.year <> case when bad.label ~ '^[0-9]{4}-[0-9]{2}$' then substring(bad.label, 1, 4)::int + 1 else bad.year end
  and exists (
    select 1 from player_seasons good
    where good.player_id = bad.player_id and good.competition = bad.competition
      and good.year = case when bad.label ~ '^[0-9]{4}-[0-9]{2}$' then substring(bad.label, 1, 4)::int + 1 else bad.year end
      and good.label = bad.label and good.team = bad.team and good.league = bad.league
      and good.games = bad.games and good.stats = bad.stats and good.conference = bad.conference
  );
-- Remove stale labels left by older label updates as well.
delete from stat_seasons s where not exists (
  select 1 from player_seasons p where p.competition = s.competition and p.year = s.year and p.label = s.label and p.league = ''
);

-- +goose Down
-- A verified duplicate-data repair cannot be usefully reversed.
select 1;
