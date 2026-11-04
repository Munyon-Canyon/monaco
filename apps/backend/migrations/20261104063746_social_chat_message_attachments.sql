ALTER TABLE cabal_messages ALTER COLUMN body DROP NOT NULL;

ALTER TABLE cabal_messages DROP CONSTRAINT cabal_messages_body_check;

ALTER TABLE cabal_messages ADD CONSTRAINT cabal_messages_body_check
  CHECK (body IS NULL OR char_length(body) BETWEEN 1 AND 2000);

CREATE TABLE chat_message_attachments (
  message_id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('gif', 'poll', 'stock', 'thesis_quote')),
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (cabal_id, message_id) REFERENCES cabal_messages (cabal_id, id) ON DELETE CASCADE
);
