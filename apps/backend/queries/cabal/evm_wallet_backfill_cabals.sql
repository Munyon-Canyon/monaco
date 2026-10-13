-- name: EVMWalletBackfillCabals :many
SELECT t.cabal_id, (e.cabal_id IS NOT NULL)::boolean AS has_evm_wallet
FROM treasury_wallets t
LEFT JOIN treasury_evm_wallets e ON e.cabal_id = t.cabal_id
ORDER BY t.cabal_id;
