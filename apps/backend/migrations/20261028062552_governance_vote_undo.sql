ALTER TABLE votes
  ADD COLUMN first_cast_at timestamptz,
  ADD COLUMN changed boolean NOT NULL DEFAULT false;

UPDATE votes SET first_cast_at = cast_at;

ALTER TABLE votes ALTER COLUMN first_cast_at SET NOT NULL;
