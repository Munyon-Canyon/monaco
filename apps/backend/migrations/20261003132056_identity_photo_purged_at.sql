ALTER TABLE users ADD COLUMN photo_purged_at timestamptz;

CREATE INDEX users_photo_purge_due_idx ON users (id) WHERE account_status = 'deleted' AND photo_purged_at IS NULL;
