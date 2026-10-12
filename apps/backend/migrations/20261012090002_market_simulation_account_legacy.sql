ALTER TABLE evm_simulation_account
  ALTER COLUMN privy_wallet_id DROP NOT NULL,
  ADD COLUMN legacy_network text CHECK (legacy_network IN ('base', 'arbitrum', 'ethereum', 'robinhood')),
  ADD CONSTRAINT evm_simulation_account_legacy_has_no_wallet
    CHECK ((legacy_network IS NULL) = (privy_wallet_id IS NOT NULL));

INSERT INTO evm_simulation_account (privy_wallet_id, address, created_at, legacy_network)
VALUES (NULL, '0xf15262a7ee9d20fa690185d58e0eb2aad475bf32', now(), 'base')
ON CONFLICT (singleton) DO NOTHING;
