package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestQueries_ownsCashOutPayoutSignatures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := adapters.NewQueries(f.pool, nil, nil, f.clock, chain.SolanaAddress(f.cfg.Solana.USDCMint))
	assertSignatureOwned(f.ctx(), t, q, "payout-signature", false)
	job, now := f.ids.NewV7(), f.clock.Now()
	if _, err := f.pool.Exec(f.ctx(), `INSERT INTO cash_out_jobs
		(id, cabal_id, user_id, share_units, payout_micros, status, created_at, updated_at)
		VALUES ($1, $2, $3, 10, 1000000, 'paying', $4, $4)`,
		job, f.cabal(t).UUID(), f.user(t).UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx(), `INSERT INTO cash_out_payouts
		(job_id, attempt, signature, signed_tx, status, created_at)
		VALUES ($1, 1, 'payout-signature', '\x01', 'signed', $2)`, job, now); err != nil {
		t.Fatal(err)
	}
	assertSignatureOwned(f.ctx(), t, q, "payout-signature", true)
	assertSignatureOwned(f.ctx(), t, q, "other-signature", false)
}
