-- name: InsertAccount :one
-- Returns no rows when the account already exists.
insert into accounts (address, type, allow_negative)
values ($1, $2, $3)
on conflict (address) do nothing
returning *;

-- name: GetAccount :one
select * from accounts where address = $1;

-- name: ListAccounts :many
-- Returns up to lim accounts whose address matches the regex pattern and sorts after after, ordered by
-- address, each with its wallets' currencies and precisions in currency order.
select a.address, a.type, a.allow_negative, a.created_at,
  coalesce(array_agg(w.currency order by w.currency) filter (where w.currency is not null), '{}')::text[] as currencies,
  coalesce(array_agg(c.precision order by w.currency) filter (where w.currency is not null), '{}')::smallint[] as precisions
from accounts a
left join wallets w on w.account_id = a.id
left join currencies c on c.name = w.currency
where a.address ~ @pattern::text and a.address > @after::text
group by a.id
order by a.address
limit @lim::int;
