ALTER TABLE proposals
  ADD COLUMN pot_value_micros bigint,
  ADD COLUMN closed_at timestamptz,
  ADD COLUMN decided_by uuid;

UPDATE proposals AS p
SET closed_at = e.at
FROM (
  SELECT aggregate_id, min(created_at) AS at
  FROM events
  WHERE aggregate_type = 'proposal'
    AND type IN ('proposal.passed', 'proposal.failed', 'proposal.expired', 'proposal.withdrawn', 'proposal.voided')
  GROUP BY aggregate_id
) AS e
WHERE p.id = e.aggregate_id AND p.status <> 'open';
