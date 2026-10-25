-- name: InsertAddress :one
-- Returns no rows when the address already exists.
insert into addresses (address, type, allow_negative)
values ($1, $2, $3)
on conflict (address) do nothing
returning *;

-- name: GetAddress :one
select * from addresses where address = $1;
