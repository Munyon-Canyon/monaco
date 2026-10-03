-- name: DeliveryRecorded :one
SELECT EXISTS (
  SELECT 1 FROM event_deliveries WHERE handler = @handler AND event_id = @event_id
)::bool AS recorded;
