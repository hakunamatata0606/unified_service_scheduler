-- name: CreateIdempotencyRequest :execrows
INSERT INTO idempotency_requests (
    key,
    request_hash,
    status,
    expires_at_utc
) VALUES (
    sqlc.arg(key),
    sqlc.arg(request_hash),
    'in_progress',
    sqlc.arg(expires_at_utc)
)
ON CONFLICT (key) DO NOTHING;

-- name: GetIdempotencyRequest :one
SELECT *
FROM idempotency_requests
WHERE key = sqlc.arg(key)
LIMIT 1;

-- name: CompleteIdempotencyRequest :one
UPDATE idempotency_requests
SET status = 'completed',
    appointment_id = sqlc.arg(appointment_id),
    response_status = sqlc.arg(response_status),
    response_body = sqlc.arg(response_body)
WHERE key = sqlc.arg(key)
  AND status = 'in_progress'
RETURNING *;

-- name: DeleteExpiredIdempotencyRequests :execrows
DELETE FROM idempotency_requests
WHERE expires_at_utc < sqlc.arg(now_utc);

