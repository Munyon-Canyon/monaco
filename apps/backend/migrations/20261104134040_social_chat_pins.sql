ALTER TABLE cabal_messages
  ADD COLUMN pinned_at timestamptz,
  ADD COLUMN pinned_by uuid REFERENCES users (id),
  ADD CONSTRAINT cabal_messages_pin_check CHECK ((pinned_at IS NULL) = (pinned_by IS NULL));

CREATE INDEX cabal_messages_pinned_idx ON cabal_messages (cabal_id, pinned_at DESC) WHERE pinned_at IS NOT NULL;

CREATE FUNCTION lock_chat_pins(cabal uuid) RETURNS void LANGUAGE sql AS $$
  SELECT pg_advisory_xact_lock(hashtextextended('chat_pins:' || cabal::text, 0))
$$;
