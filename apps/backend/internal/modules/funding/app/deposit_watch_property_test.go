package app

import (
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func TestDepositWatchCandidatesProperty_recordsEveryNonFailedSignature(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(0, 40).Draw(t, "count")
		page := make([]solana.SignatureInfo, count)
		want := make([]chain.Signature, 0, count)
		for i := range page {
			page[i] = solana.SignatureInfo{
				Signature: chain.Signature(strconv.Itoa(i)),
				Slot:      rapid.Uint64Range(0, 1<<63-1).Draw(t, "slot"),
				Failed:    rapid.Bool().Draw(t, "failed"),
			}
			if !page[i].Failed {
				want = append(want, page[i].Signature)
			}
		}
		got := watchCandidates(sqlc.DepositWatchDirtyAccountsRow{WalletAddress: "wallet"}, page)
		if len(got) != len(want) {
			t.Fatalf("candidates = %d, want %d", len(got), len(want))
		}
		for i := range got {
			if got[i].Signature != want[i] || got[i].Wallet != "wallet" || got[i].Source != "poller" {
				t.Fatalf("candidate %d = %+v, want signature %q for wallet", i, got[i], want[i])
			}
		}
	})
}
