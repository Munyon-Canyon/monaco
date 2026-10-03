ALTER TABLE cabal_txns ADD COLUMN seq bigint;
UPDATE cabal_txns c SET seq = o.seq
FROM (SELECT id, row_number() OVER (PARTITION BY cabal_id ORDER BY created_at, id) AS seq FROM cabal_txns) o
WHERE c.id = o.id;
ALTER TABLE cabal_txns ALTER COLUMN seq SET NOT NULL;
CREATE UNIQUE INDEX cabal_txns_cabal_seq ON cabal_txns (cabal_id, seq);
