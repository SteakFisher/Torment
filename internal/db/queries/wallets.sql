-- name: ListWallets :many
-- Ordered by currency.
select w.tb_id, w.currency, c.precision
from wallets w
join currencies c on c.name = w.currency
where w.account_id = $1
order by w.currency;

-- name: GetWallets :many
-- Returns one row for each (address, currency) pair whose account and currency are registered; pairs are
-- matched by position in the two arrays. tb_id and tb_created are null when the pair has no wallet.
select a.address, c.name as currency, a.id as account_id, a.type, a.allow_negative, c.ledger, c.precision, w.tb_id, w.tb_created
from (select unnest(@addresses::text[]) as address, unnest(@currencies::text[]) as currency) p
join accounts a on a.address = p.address
join currencies c on c.name = p.currency
left join wallets w on w.account_id = a.id and w.currency = c.name;

-- name: InsertWallets :exec
-- Inserts one wallet per position in the three arrays, skipping any the account already has in the currency.
insert into wallets (tb_id, account_id, currency)
select unnest(@tb_ids::bytea[]), unnest(@account_ids::bigint[]), unnest(@currencies::text[])
on conflict (account_id, currency) do nothing;

-- name: MarkWalletsTBCreated :exec
update wallets set tb_created = true where tb_id = any(@tb_ids::bytea[]);
