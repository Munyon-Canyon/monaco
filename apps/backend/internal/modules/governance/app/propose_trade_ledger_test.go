package app

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestSpendableCashIsLedgerCashNetOfReservationsAndNeverMoreThanTheChain(t *testing.T) {
	t.Parallel()
	type tc struct{ onChain, booked, reserved, want uint64 }
	for name, c := range map[string]tc{
		"ledger below the chain":         {1_200_000_000, 700_000_000, 0, 700_000_000},
		"a dust stray on the chain":      {100_000_001, 100_000_000, 0, 100_000_000},
		"chain below the ledger":         {300_000_000, 700_000_000, 0, 300_000_000},
		"reservation shrinks the ledger": {900_000_000, 700_000_000, 200_000_000, 500_000_000},
		"reservation over the ledger":    {900_000_000, 700_000_000, 800_000_000, 0},
	} {
		ports, id := capPorts(c.onChain, solana.MintConfig{}, nil)
		treasury := fakes.NewTreasury()
		treasury.SetCash(id, capUSDC, c.booked)
		treasury.SetCashOutReservation(id, money.MicrosFromUint64(c.reserved))
		ports.Treasury = treasury
		got, _, err := spendableCash(context.Background(), ports, id, marketfake.AAPLx())
		if err != nil || got.Uint64() != c.want {
			t.Errorf("%s: spendable cash = %s, %v; want %d", name, got, err, c.want)
		}
	}
}

func TestAMaxBuyOfLedgerCashPlusOneMicroIsRefused(t *testing.T) {
	t.Parallel()
	ports, id := capPorts(100_000_001, solana.MintConfig{}, nil)
	treasury := fakes.NewTreasury()
	treasury.SetCash(id, capUSDC, 100_000_000)
	ports.Treasury = treasury
	trade := capTrade(domain.KindBuy, 100_000_001)
	if err := funds(
		context.Background(),
		ports,
		id,
		marketfake.AAPLx(),
		trade,
		money.MicrosFromUint64(1<<62),
	); err == nil {
		t.Fatal("a buy of ledger cash + 1 micro passed the funds check")
	}
	trade = capTrade(domain.KindBuy, 100_000_000)
	if err := funds(
		context.Background(),
		ports,
		id,
		marketfake.AAPLx(),
		trade,
		money.MicrosFromUint64(1<<62),
	); err != nil {
		t.Fatalf("a buy of exactly the ledger cash = %v, want nil", err)
	}
}
