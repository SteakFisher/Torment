-- +goose Up
create type account_type as enum ('asset', 'expense', 'liability', 'equity', 'revenue');

create table accounts (
  id             bigint generated always as identity primary key,
  address        text not null unique,
  type           account_type not null,
  allow_negative boolean not null default false,
  created_at     timestamptz not null default now()
);

-- serves prefix lookups (address like 'rider:%')
create index accounts_address_prefix_idx on accounts (address text_pattern_ops);

-- +goose Down
drop table accounts;
drop type account_type;
