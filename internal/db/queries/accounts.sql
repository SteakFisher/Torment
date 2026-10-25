-- name: InsertAccount :one
-- Returns no rows when the account already exists.
insert into accounts (address, type, allow_negative)
values ($1, $2, $3)
on conflict (address) do nothing
returning *;

-- name: GetAccount :one
select * from accounts where address = $1;
