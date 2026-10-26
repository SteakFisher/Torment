-- +goose Up
create table wallets (
  tb_id      bytea primary key check (length(tb_id) = 16), -- TigerBeetle account id
  account_id bigint not null references accounts(id),
  currency   text not null references currencies(name),
  created_at timestamptz not null default now(),
  unique (account_id, currency)
);

-- +goose Down
drop table wallets;
