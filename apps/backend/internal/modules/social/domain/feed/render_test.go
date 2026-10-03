package feed_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const golden = "render.golden"

type renderCase struct {
	name    string
	kind    feed.Kind
	payload feed.Payload
}

func usd(micros uint64) money.Micros { return money.MicrosFromUint64(micros) }

func renderCases() []renderCase {
	alpha := "Alpha Cabal"
	return []renderCase{
		{"proposal_buy", feed.KindProposal, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", Action: feed.ActionBuy, USDCMicros: usd(500_000_000), VoterCount: 3,
		}},
		{"proposal_buy_cents", feed.KindProposal, feed.Payload{
			CabalName: alpha, Symbol: "TSLAx", Action: feed.ActionBuy, USDCMicros: usd(1_234_567_891), VoterCount: 1,
		}},
		{"proposal_sell_no_amount", feed.KindProposal, feed.Payload{
			CabalName: alpha, Symbol: "NVDAx", Action: feed.ActionSell, VoterCount: 4, YesVotes: 2, NoVotes: 1,
		}},
		{"proposal_sell_amount", feed.KindProposal, feed.Payload{
			CabalName: alpha, Symbol: "NVDAx", Action: feed.ActionSell, USDCMicros: usd(25_050_000),
			VoterCount: 1, YesVotes: 1,
		}},
		{"trade_buy", feed.KindTrade, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: usd(12_500_000_000),
		}},
		{"trade_sell", feed.KindTrade, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", Action: feed.ActionSell, USDCMicros: usd(99_990_000),
		}},
		{"price_move_up", feed.KindPriceMove, feed.Payload{
			Symbol: "AAPLx", AssetName: "Apple", ChangeBps: 1043, PriceMicros: usd(187_420_000),
		}},
		{"price_move_down", feed.KindPriceMove, feed.Payload{
			Symbol: "TSLAx", AssetName: "Tesla", ChangeBps: -507, PriceMicros: usd(1_250_005_000),
		}},
		{"price_move_flat_no_price", feed.KindPriceMove, feed.Payload{Symbol: "AAPLx"}},
		{"cabal_created", feed.KindCabalCreated, feed.Payload{CabalName: alpha, ActorName: "alice"}},
		{"cabal_created_no_actor", feed.KindCabalCreated, feed.Payload{CabalName: alpha}},
		{"member_joined", feed.KindMemberJoined, feed.Payload{CabalName: alpha, ActorName: "bob"}},
		{"member_joined_no_actor", feed.KindMemberJoined, feed.Payload{CabalName: alpha}},
		{"unknown_kind", feed.Kind("news"), feed.Payload{Symbol: "AAPLx", ChangeBps: 100}},
	}
}

func TestRender_matchesTheGoldenForEveryCase(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for _, c := range renderCases() {
		b.WriteString(strings.Join([]string{
			c.name,
			feed.RenderTitle(c.kind, c.payload),
			feed.RenderDetail(c.kind, c.payload),
			string(feed.RenderTone(c.kind, c.payload)),
		}, "\t") + "\n")
	}
	if *update {
		if err := os.WriteFile(filepath.Join("testdata", golden), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(os.DirFS("testdata"), golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/modules/social/domain/feed -update)", err)
	}
	if got := b.String(); got != string(want) {
		t.Fatalf("render drifted from testdata/%s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

func TestRender_coversEveryKind(t *testing.T) {
	t.Parallel()
	seen := map[feed.Kind]bool{}
	for _, c := range renderCases() {
		seen[c.kind] = true
	}
	for _, k := range feed.Kinds() {
		if !seen[k] {
			t.Errorf("no golden case for %s", k)
		}
	}
}

func TestRender_neverWritesBannedWords(t *testing.T) {
	t.Parallel()
	for _, c := range renderCases() {
		text := strings.ToLower(feed.RenderTitle(c.kind, c.payload) + " " + feed.RenderDetail(c.kind, c.payload))
		for _, banned := range []string{"xstock", "mint", "club", "group", "wallet"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s renders %q, which says %q", c.name, text, banned)
			}
		}
	}
}
