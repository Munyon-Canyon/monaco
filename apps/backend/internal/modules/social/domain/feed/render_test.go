package feed_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
			CabalName: alpha, ActorName: "alice", Symbol: "AAPLx", Action: feed.ActionBuy,
			USDCMicros: usd(500_000_000), Status: "open",
		}},
		{"proposal_buy_cents", feed.KindProposal, feed.Payload{
			CabalName: alpha, ActorName: "alice", Symbol: "TSLAx", Action: feed.ActionBuy,
			USDCMicros: usd(1_234_567_891), Status: "passed",
		}},
		{"proposal_sell_no_amount", feed.KindProposal, feed.Payload{
			CabalName: alpha, ActorName: "bob", Symbol: "NVDAx", Action: feed.ActionSell, Status: "executed",
		}},
		{"proposal_sell_amount", feed.KindProposal, feed.Payload{
			CabalName: alpha, ActorName: "bob", Symbol: "NVDAx", Action: feed.ActionSell,
			USDCMicros: usd(25_050_000), Status: "execution_failed",
		}},
		{"proposal_blocked", feed.KindProposal, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", Action: feed.ActionBuy, USDCMicros: usd(500_000_000),
			Status: "execution_blocked", StatusCode: string(errs.CodeFeedItemPending),
		}},
		{"proposal_failed", feed.KindProposal, feed.Payload{CabalName: alpha, Symbol: "AAPLx", Status: "failed"}},
		{"proposal_expired", feed.KindProposal, feed.Payload{CabalName: alpha, Symbol: "AAPLx", Status: "expired"}},
		{"proposal_withdrawn", feed.KindProposal, feed.Payload{CabalName: alpha, Symbol: "AAPLx", Status: "withdrawn"}},
		{"proposal_voided", feed.KindProposal, feed.Payload{CabalName: alpha, Symbol: "AAPLx", Status: "voided"}},
		{"proposal_unknown_status", feed.KindProposal, feed.Payload{CabalName: alpha, Symbol: "AAPLx"}},
		{"trade_buy", feed.KindTrade, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: usd(12_500_000_000),
		}},
		{"trade_buy_filled", feed.KindTrade, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: usd(500_000_000), PriceMicros: usd(212_400_000),
		}},
		{"trade_sell", feed.KindTrade, feed.Payload{
			CabalName: alpha, Symbol: "AAPLx", Action: feed.ActionSell, USDCMicros: usd(99_990_000),
		}},
		{"price_move_up_500", feed.KindPriceMove, feed.Payload{
			Symbol: "AAPLx", ChangeBps: 500, MarkMicros: usd(212_400_000), PrevClose: usd(193_100_000),
		}},
		{"price_move_down_1000", feed.KindPriceMove, feed.Payload{
			Symbol: "TSLAx", ChangeBps: -1000, MarkMicros: usd(1_250_005_000), PrevClose: usd(1_388_900_000),
		}},
		{"price_move_up_1234", feed.KindPriceMove, feed.Payload{
			Symbol: "AAPLx", ChangeBps: 1234, MarkMicros: usd(212_400_000), PrevClose: usd(189_000_000),
		}},
		{"price_move_no_prev_close", feed.KindPriceMove, feed.Payload{
			Symbol: "AAPLx", ChangeBps: 500, MarkMicros: usd(212_400_000),
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

func TestProposalStatus_movesForwardOnly(t *testing.T) {
	t.Parallel()
	terminal := []string{"open", "passed", "execution_failed"}
	want := map[feed.ProposalStatus][]string{
		feed.StatusOpen:             nil,
		feed.StatusPassed:           {"open"},
		feed.StatusExecutionFailed:  {"open", "passed"},
		feed.StatusFailed:           terminal,
		feed.StatusExpired:          terminal,
		feed.StatusWithdrawn:        terminal,
		feed.StatusVoided:           terminal,
		feed.StatusExecuted:         terminal,
		feed.StatusExecutionBlocked: terminal,
	}
	if got := feed.ProposalStatus("unknown").MovesFrom(); !slices.Equal(got, terminal) {
		t.Errorf("unknown.MovesFrom() = %v, want %v", got, terminal)
	}
	for to, from := range want {
		if got := to.MovesFrom(); !slices.Equal(got, from) {
			t.Errorf("%s.MovesFrom() = %v, want %v", to, got, from)
		}
	}
}
