-- +goose Up

-- A franchise that has turned auto pick on for a draft. Its picks are made
-- for it a few seconds after they come up: the top of its queue, or failing
-- that one of the best players left.
create table draft_autopick (
  draft_id     uuid not null references drafts(id) on delete cascade,
  franchise_id uuid not null references franchises(id),
  primary key (draft_id, franchise_id)
);

-- +goose Down
drop table draft_autopick;
