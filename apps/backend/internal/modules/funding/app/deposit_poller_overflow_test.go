package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func TestCheckpointPageRejectsAnOverflowingBackfillHead(t *testing.T) {
	t.Parallel()
	poller := &DepositPoller{}
	page := make([]solana.SignatureInfo, depositSignaturePageSize)
	if err := poller.checkpointPage(
		t.Context(), nil, chain.SolanaAddress("wallet"), page, backfillCursor{slot: math.MaxUint64},
	); err == nil {
		t.Fatal("checkpointPage error = nil")
	}
}

func TestCompletePageRejectsAnOverflowingBackfillHead(t *testing.T) {
	t.Parallel()
	poller := &DepositPoller{}
	if err := poller.completePage(
		t.Context(), nil, chain.SolanaAddress("wallet"), backfillCursor{slot: math.MaxUint64},
	); err == nil {
		t.Fatal("completePage error = nil")
	}
}
