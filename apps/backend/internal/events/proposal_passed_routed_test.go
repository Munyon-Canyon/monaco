package events_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const routedPassedGolden = "routed/proposal.passed.v1.json"

func routedPassedFixture(t *testing.T) events.ProposalPassed {
	t.Helper()
	g := testkit.NewIDs(1103)
	listing := func() multichain.ListingID {
		var id multichain.ListingID
		if err := id.UnmarshalText([]byte(g.NewV7().String())); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(g.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	base, solana := listing(), listing()
	at := time.Date(2026, 10, 11, 8, 50, 0, 0, time.UTC)
	quote := func(
		id multichain.ListingID, n chain.Network, v multichain.Venue, out int64, shares multichain.ShareNanos,
		costs []multichain.Cost,
	) multichain.Quote {
		return multichain.Quote{
			ListingID: id, Network: n, Venue: v, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(100_000_000), GrossOutput: big.NewInt(out), Costs: costs,
			NetShares: shares, NetMicros: money.MicrosFromUint64(100_000_000),
			QuotedAt: at, ExpiresAt: at.Add(30 * time.Second), RouteID: "quote-" + string(n),
		}
	}
	return events.ProposalPassed{
		V: 1, ProposalID: g.NewV7(), CabalID: g.NewV7(), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
		USDCMicros: money.MicrosFromUint64(100_000_000), QuoteOutAmount: 29_610_838, ProposerID: g.NewV7(),
		PriceComparison: &multichain.Comparison{
			StockID: stock, Side: multichain.SideBuy, Winner: base,
			Ranked: []multichain.Quote{
				quote(base, chain.NetworkBase, multichain.VenueUniswap, 30_690_575, 306_675_000, []multichain.Cost{
					{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(75_000)},
					{Kind: multichain.CostGas, Micros: money.MicrosFromUint64(76_119)},
				}),
				quote(solana, chain.NetworkSolana, multichain.VenueJupiter, 29_610_838, 297_076_000, nil),
			},
		},
	}
}

func TestProposalPassedRoutedPayloadMatchesGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(routedPassedFixture(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Join(goldenDir, "routed"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(goldenDir, routedPassedGolden), got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(golden(), routedPassedGolden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/events -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("routed proposal.passed drifted from %s\n got: %s\nwant: %s", routedPassedGolden, got, want)
	}
}

func TestProposalPassedRoutedGoldenDecodesToTheFixture(t *testing.T) {
	t.Parallel()
	payload, err := fs.ReadFile(golden(), routedPassedGolden)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := events.Decode(events.TypeProposalPassed, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if want := routedPassedFixture(t); !reflect.DeepEqual(ev, want) {
		t.Errorf("Decode = %+v, want %+v", ev, want)
	}
}
