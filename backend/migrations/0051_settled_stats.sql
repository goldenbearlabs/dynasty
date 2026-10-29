-- +goose Up
-- When a game went final. Feeds fill in a finished game's last details (a
-- goalie's decision, a shutout, an overtime goal) some minutes after the
-- score says final, so its box score is fetched again for a while after.
alter table games add column final_at timestamptz;
update games set final_at = starts_at + interval '4 hours' where status = 'final';

-- The first day a starter's games may count in a slot that counts only some
-- games a week: the day he came into the slot. Null is from the start of the
-- week. A game picked for him that he does not play in gives way to one
-- after it, or failing that one before it, but never one from before then.
alter table lineup_entries add column joined_on date;

-- +goose Down
alter table lineup_entries drop column joined_on;
alter table games drop column final_at;
