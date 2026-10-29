ALTER TABLE proposals ADD COLUMN rule_change jsonb;

ALTER TABLE proposals ALTER COLUMN symbol DROP NOT NULL;
ALTER TABLE proposals ALTER COLUMN mint DROP NOT NULL;
ALTER TABLE proposals ALTER COLUMN quote_out_amount DROP NOT NULL;

ALTER TABLE proposals DROP CONSTRAINT proposals_one_amount_check;

ALTER TABLE proposals ADD CONSTRAINT proposals_one_amount_check CHECK (
  (
    kind <> 'rule'
    AND rule_change IS NULL
    AND symbol IS NOT NULL
    AND mint IS NOT NULL
    AND quote_out_amount IS NOT NULL
    AND (
      (usdc_micros IS NOT NULL AND usdc_micros > 0 AND token_amount IS NULL)
      OR (token_amount IS NOT NULL AND token_amount > 0 AND usdc_micros IS NULL)
    )
  )
  OR (
    kind = 'rule'
    AND rule_change IS NOT NULL
    AND usdc_micros IS NULL
    AND token_amount IS NULL
    AND symbol IS NULL
    AND mint IS NULL
    AND quote_out_amount IS NULL
  )
);

CREATE UNIQUE INDEX proposals_open_rule_cabal_uq ON proposals (cabal_id, (rule_change ->> 'rule'))
  WHERE status = 'open' AND kind = 'rule' AND rule_change ->> 'rule' NOT IN ('voter', 'admin');

CREATE UNIQUE INDEX proposals_open_rule_member_uq ON proposals (cabal_id, (rule_change ->> 'rule'), (rule_change ->> 'user_id'))
  WHERE status = 'open' AND kind = 'rule' AND rule_change ->> 'rule' IN ('voter', 'admin');
