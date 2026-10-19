-- +goose Up
-- A matchup the commissioner has doubled, a long one over a holiday: in it
-- each slot that counts only some games a week counts twice as many, and a
-- team may make twice its weekly pitcher starts, all over the whole matchup.
alter table periods add column double_games boolean not null default false;

-- +goose Down
alter table periods drop column double_games;
