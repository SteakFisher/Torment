-- name: InsertCurrency :one
insert into currencies (name, precision) values ($1, $2) returning *;

-- name: GetCurrencies :many
select * from currencies where name = any(@names::text[]);
