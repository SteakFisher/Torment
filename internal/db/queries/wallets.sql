-- name: InsertWallet :one
-- Returns no rows when the account already has a wallet in the currency.
insert into wallets (tb_id, account_id, currency)
values ($1, $2, $3)
on conflict (account_id, currency) do nothing
returning *;

-- name: GetWallet :one
select * from wallets where account_id = $1 and currency = $2;

-- name: ListWallets :many
-- Ordered by currency.
select w.tb_id, w.currency, c.precision
from wallets w
join currencies c on c.name = w.currency
where w.account_id = $1
order by w.currency;

-- name: GetWalletsByAddress :many
-- Returns one row for each (address, currency) pair that has a wallet; pairs are matched by position in
-- the two arrays.
select a.address, w.currency, w.tb_id, c.ledger
from accounts a
join wallets w on w.account_id = a.id
join currencies c on c.name = w.currency
where (a.address, w.currency) in (select unnest(@addresses::text[]), unnest(@currencies::text[]));
