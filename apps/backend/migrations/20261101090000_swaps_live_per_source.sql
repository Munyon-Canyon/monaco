DROP INDEX swaps_live_source_idx;

CREATE UNIQUE INDEX swaps_live_source_idx ON swaps (source_kind, source_id, in_mint)
  WHERE status <> 'failed' AND source_kind <> 'proposal';

CREATE UNIQUE INDEX swaps_live_proposal_idx ON swaps (source_id)
  WHERE status <> 'failed' AND source_kind = 'proposal';
