-- name: InsertCurrency :one
insert into currencies (name, precision, ledger) values ($1, $2, $3) returning *;
