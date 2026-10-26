-- +goose Up
create table currencies (
  name      text primary key,
  precision smallint not null check (precision between 0 and 18),
  ledger    integer not null generated always as identity unique -- TigerBeetle ledger number
);

-- +goose Down
drop table currencies;
