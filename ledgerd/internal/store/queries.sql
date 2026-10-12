-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: AddToBalance :execrows
UPDATE accounts
SET balance = balance + sqlc.arg(delta)
WHERE id = sqlc.arg(id);
