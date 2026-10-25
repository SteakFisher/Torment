-- name: InsertAsset :one
insert into assets (name, precision, ledger) values ($1, $2, $3) returning *;
