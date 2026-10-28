ALTER TABLE notifications
  ADD COLUMN thread_id text,
  ADD COLUMN category text,
  ADD COLUMN interruption_level text,
  ADD COLUMN presentation jsonb;
