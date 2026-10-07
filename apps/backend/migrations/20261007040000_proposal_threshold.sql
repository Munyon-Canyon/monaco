ALTER TABLE proposals ADD COLUMN threshold text;
UPDATE proposals SET threshold = cabals.threshold FROM cabals WHERE cabals.id = proposals.cabal_id;
ALTER TABLE proposals ALTER COLUMN threshold SET NOT NULL;
ALTER TABLE proposals ADD CONSTRAINT proposals_threshold_check CHECK (threshold IN ('majority', 'unanimous'));
