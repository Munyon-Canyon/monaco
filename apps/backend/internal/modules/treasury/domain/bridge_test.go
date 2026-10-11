package domain_test

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestBridgeTxns_refuseAFeeAboveTheSentAmountAndAmountsPastInt64(t *testing.T) {
	t.Parallel()
	id := ids.Real{}.NewV7()
	bridge := func(sent, fee uint64) domain.Bridge {
		return domain.Bridge{
			SwapID: id, CabalID: cabalID(t, 7), Direction: domain.BridgeOut, Network: chain.NetworkBase,
			Sent: money.MicrosFromUint64(sent), Fee: money.MicrosFromUint64(fee),
		}
	}
	for name, build := range map[string]func() (domain.CabalTxn, error){
		"fee above sent": func() (domain.CabalTxn, error) {
			return domain.NewBridgeLandedTxn(id, bridge(10, 11), usdc)
		},
		"debit past int64": func() (domain.CabalTxn, error) {
			return domain.NewBridgeSentTxn(id, bridge(math.MaxInt64+2, 0), usdc)
		},
		"credit past int64": func() (domain.CabalTxn, error) {
			return domain.NewBridgeReturnedTxn(id, bridge(math.MaxInt64+1, 0), usdc)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := build()
			wantCode(t, err, errs.CodeInvalidInput)
		})
	}
}

func TestPositionDeltas_refusesCashOutflowsSummingPastUint64(t *testing.T) {
	t.Parallel()
	cash := []domain.Asset{usdc, domain.EVMCashAsset(chain.NetworkBase), domain.EVMCashAsset(chain.NetworkArbitrum)}
	entries := make([]domain.CabalEntry, 0, 3*len(cash))
	for _, asset := range cash {
		entries = append(entries,
			leg(domain.CabalTreasury, asset, math.MinInt64),
			leg(domain.CabalVenue, asset, 1<<62), leg(domain.CabalVenue, asset, 1<<62))
	}
	_, err := cabalTxn(t, entries...).PositionDeltas(usdc)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestBridgeTxnID_keepsTheFirstAttemptsIDAndGivesEachRetryItsOwn(t *testing.T) {
	t.Parallel()
	swap := ids.Real{}.NewV7()
	attempt := func(n int) domain.Bridge {
		return domain.Bridge{SwapID: swap, Direction: domain.BridgeBack, Network: chain.NetworkBase, Attempt: n}
	}
	first := uuid.NewSHA1(swap, []byte("bridge:bridge_back:succeeded"))
	if attempt(0).TxnID("succeeded") != first || attempt(1).TxnID("succeeded") != first {
		t.Fatalf(
			"first attempt ids = %s, %s, want %s",
			attempt(0).TxnID("succeeded"),
			attempt(1).TxnID("succeeded"),
			first,
		)
	}
	if got, want := attempt(
		2,
	).TxnID("succeeded"),
		uuid.NewSHA1(
			swap,
			[]byte("bridge:bridge_back:succeeded:2"),
		); got != want {
		t.Fatalf("attempt 2 id = %s, want %s", got, want)
	}
	if attempt(2).TxnID("succeeded") == attempt(3).TxnID("succeeded") {
		t.Fatal("attempts 2 and 3 share a ledger id")
	}
}
