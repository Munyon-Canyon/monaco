package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	fundingadapters "github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type usdcIn struct{ usdc chain.SolanaAddress }

func (r usdcIn) InboundTransfersForMint(
	context.Context, chain.Signature, chain.SolanaAddress, chain.SolanaAddress,
) ([]solana.Transfer, error) {
	return []solana.Transfer{{
		Mint: chain.Mint{Address: r.usdc, Decimals: 6}, Amount: money.NewBaseUnits(25_000_000, 6),
		Net: money.NewBaseUnits(25_000_000, 6),
	}}, nil
}

func TestADatabaseBlipDuringTheOwnershipCheckIsRetryable(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	down := testkit.DB(t)
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	deps := module.Deps{
		Config: cfg, Pool: down, IDs: testkit.NewIDs(1),
		Clock: testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
	}
	usdc := chain.SolanaAddress(cfg.Solana.USDCMint)
	owners := []port.SignatureOwner{treasury.New(deps).SignatureOwner(), fundingadapters.NewBounceSignatures(down)}
	r := app.NewDepositCandidateResolver(usdcIn{usdc: usdc}, usdc, owners, nil, deps.IDs)

	_, err := r.Fetch(gone, events.DepositCandidateSeen{V: 1, TxSignature: "sig", WalletAddress: "wallet"})

	if code := db.CodeOf(err); err == nil || errs.VerdictFor(code) != errs.VerdictNak {
		t.Fatalf("Fetch with the database unreachable: err %v, code %s; want an error that naks", err, code)
	}
}
