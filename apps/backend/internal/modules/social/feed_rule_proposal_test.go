package social_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestFeedRuleProposal_aRuleCreatedFailedAndExpiredAckWithNoFeedRow(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	delete(r.tallies.votes, ids.ProposalIDFrom(r.proposal))
	r.mustCreate(t, func(e *events.ProposalCreated) { e.Kind = "rule" })
	closes := map[string]func(context.Context, db.Tx) error{
		"failed": func(ctx context.Context, tx db.Tx) error {
			e := events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal, Kind: "rule"}
			closed, err := r.feed.FetchFailed(ctx, e)
			if err != nil {
				return err
			}
			return r.feed.ApplyFailed(ctx, tx, e, closed, r.clock.Now())
		},
		"expired": func(ctx context.Context, tx db.Tx) error {
			e := events.ProposalExpired{V: 1, ProposalID: r.proposal, CabalID: r.cabal, Kind: "rule"}
			closed, err := r.feed.FetchExpired(ctx, e)
			if err != nil {
				return err
			}
			return r.feed.ApplyExpired(ctx, tx, e, closed, r.clock.Now())
		},
	}
	for name, closeFn := range closes {
		if err := r.deliver(t, closeFn); err != nil {
			t.Fatalf("%s: %v, want an ack", name, err)
		}
	}
	if n := r.count(t, "proposal") + r.count(t, "result"); n != 0 {
		t.Fatalf("%d proposal and result rows for a rule proposal, want none", n)
	}
}

func TestFeedRuleProposal_aBuyFailedStillMakesItsResult(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.failed(t); err != nil {
		t.Fatal(err)
	}
	if n := r.count(t, "result"); n != 1 {
		t.Fatalf("%d result rows for a failed buy, want 1", n)
	}
}
