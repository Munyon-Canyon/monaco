package events_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const buyPassedGolden = "buy/proposal.passed.v1.json"

func buyPassedFixture() events.ProposalPassed {
	g := testkit.NewIDs(1102)
	return events.ProposalPassed{
		V: 1, ProposalID: g.NewV7(), CabalID: g.NewV7(), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
		USDCMicros: money.MicrosFromUint64(25_000_000), QuoteOutAmount: 105_000_000, ProposerID: g.NewV7(),
	}
}

func TestProposalPassedBuyPayloadMatchesGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(buyPassedFixture(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(filepath.Join(goldenDir, buyPassedGolden), got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(golden(), buyPassedGolden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/events -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("buy proposal.passed drifted from %s\n got: %s\nwant: %s", buyPassedGolden, got, want)
	}
}

func TestProposalPassedBuyGoldenDecodesToTheFixture(t *testing.T) {
	t.Parallel()
	payload, err := fs.ReadFile(golden(), buyPassedGolden)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := events.Decode(events.TypeProposalPassed, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if want := buyPassedFixture(); !reflect.DeepEqual(ev, want) {
		t.Errorf("Decode = %+v, want %+v", ev, want)
	}
}
