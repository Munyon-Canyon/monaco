CREATE INDEX feed_objects_actor_created_idx ON feed_objects (actor_id, created_at DESC, id DESC);
