-- +goose Up
create table kinds (
  name text primary key,
  code integer not null generated always as identity (maxvalue 65535) unique -- TigerBeetle transfer code
);

-- +goose Down
drop table kinds;
