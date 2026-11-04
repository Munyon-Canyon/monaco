//go:build faultpoints

package social_test

import (
	"context"
	"testing"

	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow22e_WriteChatProposalMessage_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	e := f.proposalEvent("buy")
	heads := stubHeads{heads: map[ids.ProposalID]governanceport.ProposalHead{
		ids.ProposalIDFrom(e.ProposalID): {Kind: e.Kind, Symbol: e.Symbol, Thesis: proposalThesis},
	}}
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		return f.proposalWriter(heads).Handle(ctx, e)
	})
	if rows := f.proposalRows(t); len(rows) != 1 {
		t.Fatalf("rows = %+v, want one proposal row after the restart", rows)
	}
	if got := len(f.rt.Published()); got != 1 {
		t.Fatalf("%d messages published, want 1: nothing went out before the commit", got)
	}
}
