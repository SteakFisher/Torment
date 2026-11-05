-- name: InsertCurrency :one
-- Returns no rows when the currency already exists.
insert into currencies (name, precision) values ($1, $2)
on conflict (name) do nothing
returning *;

-- name: GetCurrency :one
select * from currencies where name = $1;

-- name: GetCurrencies :many
select * from currencies where name = any(@names::text[]);

-- name: ListCurrencies :many
select * from currencies order by name;
