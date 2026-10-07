-- name: DeadLetterSeen :one
SELECT EXISTS (SELECT 1 FROM dead_letters WHERE stream_seq = sqlc.arg(stream_seq));

-- name: UpsertDeadLetter :one
INSERT INTO dead_letters (
  id, stream_seq, consumer, handler, subject, event_id, code, error, letter, occurrences, status,
  first_seen_at, last_seen_at
)
VALUES (
  sqlc.arg(id), sqlc.arg(stream_seq), sqlc.arg(consumer), sqlc.arg(handler), sqlc.arg(subject),
  NULLIF(sqlc.arg(event_id)::text, '')::uuid, sqlc.arg(code), sqlc.arg(error), sqlc.arg(letter), 1, 'open',
  sqlc.arg(seen_at), sqlc.arg(seen_at)
)
ON CONFLICT (consumer, event_id) WHERE event_id IS NOT NULL AND status IN ('open', 'redriven') DO UPDATE SET
  stream_seq = EXCLUDED.stream_seq,
  occurrences = dead_letters.occurrences + 1,
  last_seen_at = EXCLUDED.last_seen_at,
  code = EXCLUDED.code,
  error = EXCLUDED.error,
  letter = EXCLUDED.letter,
  status = 'open'
RETURNING id, occurrences, status;

-- name: ResolveDeadLetter :execrows
UPDATE dead_letters
SET status = 'resolved', resolved_at = sqlc.arg(resolved_at)::timestamptz
WHERE consumer = sqlc.arg(consumer) AND event_id = sqlc.arg(event_id)::uuid AND status IN ('open', 'redriven');

-- name: CountLiveDeadLetters :many
SELECT consumer, count(*)::bigint AS live
FROM dead_letters
WHERE status IN ('open', 'redriven')
GROUP BY consumer;
