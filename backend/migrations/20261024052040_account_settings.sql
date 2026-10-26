-- Stores short-lived email-change challenges without changing an account's current address.

-- +goose Up
create table account_email_changes (
  user_id uuid primary key references users(id) on delete cascade,
  new_email text not null,
  code_hash bytea not null,
  expires_at timestamptz not null,
  attempts int not null default 0,
  requested_at timestamptz not null default now()
);

-- +goose Down
drop table account_email_changes;
