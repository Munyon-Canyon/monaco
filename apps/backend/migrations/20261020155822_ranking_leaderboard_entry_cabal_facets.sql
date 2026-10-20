ALTER TABLE leaderboard_entries
  ADD COLUMN join_mode text,
  ADD COLUMN member_count int NOT NULL DEFAULT 0;

UPDATE leaderboard_entries e
SET join_mode = c.join_mode,
  member_count = (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)
FROM cabals c
WHERE e.board = 'cabals' AND e.subject_id = c.id;
