CREATE INDEX cabal_txns_tx_signature_idx ON cabal_txns (tx_signature) WHERE tx_signature IS NOT NULL;
CREATE INDEX user_txns_tx_signature_idx ON user_txns (tx_signature) WHERE tx_signature IS NOT NULL;
