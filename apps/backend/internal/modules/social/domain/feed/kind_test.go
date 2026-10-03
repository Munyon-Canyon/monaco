package feed_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func TestParseKind_acceptsEveryKindAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	for _, want := range feed.Kinds() {
		got, err := feed.ParseKind(string(want))
		if err != nil || got != want {
			t.Fatalf("ParseKind(%q) = %q, %v", want, got, err)
		}
	}
	for _, raw := range []string{"", "news", "Proposal", "trade "} {
		if _, err := feed.ParseKind(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("ParseKind(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestKindRefType_namesTheSourceTableOfEachKind(t *testing.T) {
	t.Parallel()
	want := map[feed.Kind]feed.RefType{
		feed.KindProposal:     feed.RefProposals,
		feed.KindTrade:        feed.RefSwaps,
		feed.KindPriceMove:    feed.RefAssetPriceMoves,
		feed.KindCabalCreated: feed.RefCabals,
		feed.KindMemberJoined: feed.RefCabalMembers,
	}
	if len(want) != len(feed.Kinds()) {
		t.Fatalf("table covers %d kinds, want %d", len(want), len(feed.Kinds()))
	}
	for _, k := range feed.Kinds() {
		if got := k.RefType(); got != want[k] {
			t.Errorf("%s.RefType() = %q, want %q", k, got, want[k])
		}
	}
	if got := feed.Kind("news").RefType(); got != "" {
		t.Fatalf("news.RefType() = %q, want empty", got)
	}
}

func TestParseScope_defaultsToAllAndRefusesUnknownScopes(t *testing.T) {
	t.Parallel()
	valid := map[string]feed.Scope{"": feed.ScopeAll, "all": feed.ScopeAll, "following": feed.ScopeFollowing}
	for raw, want := range valid {
		if got, err := feed.ParseScope(raw); err != nil || got != want {
			t.Errorf("ParseScope(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"mine", "bogus", "All"} {
		if _, err := feed.ParseScope(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseScope(%q) err = %v, want invalid_input", raw, err)
		}
	}
}
