-- name: InsertKind :one
-- Returns no rows when the kind already exists, and then takes no code from the identity sequence.
insert into kinds (name)
select @name::text
where not exists (select 1 from kinds where name = @name::text)
on conflict (name) do nothing
returning *;

-- name: GetKind :one
select * from kinds where name = $1;

-- name: GetKinds :many
select * from kinds where name = any(@names::text[]);

-- name: GetKindsByCodes :many
select * from kinds where code = any(@codes::integer[]);

-- name: ListKinds :many
select * from kinds order by name;
