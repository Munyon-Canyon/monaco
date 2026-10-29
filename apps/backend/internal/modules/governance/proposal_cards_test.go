package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func (f pendingFacts) decide(t *testing.T, proposal, by uuid.UUID) {
	t.Helper()
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE proposals SET decided_by = $2 WHERE id = $1`,
		proposal,
		by,
	); err != nil {
		t.Fatal(err)
	}
}

type closedCards struct {
	closed, untouched port.CatchUpProposal
	decider           uuid.UUID
}

func readClosedCards(t *testing.T) closedCards {
	t.Helper()
	f := newPendingFacts(t)
	mia, kai, jo := f.ids.NewV7(), f.ids.NewV7(), f.ids.NewV7()
	closed := f.open(t, f.ids.NewV7(), mia, time.Hour, f.caller, mia, kai, jo)
	f.insert(t, closed)
	f.ballot(t, closed.ID, f.caller, "yes")
	f.ballot(t, closed.ID, mia, "yes")
	f.ballot(t, closed.ID, kai, "yes")
	f.ballot(t, closed.ID, jo, "no")
	f.setStatus(t, closed.ID, string(domain.StatusPassed), "")
	f.decide(t, closed.ID, kai)
	untouched := f.open(t, f.ids.NewV7(), kai, 2*time.Hour, f.caller, mia)
	f.insert(t, untouched)
	f.names.set(closed.CabalID, "Degens", cabalport.StatusActive)
	f.names.set(untouched.CabalID, "AC Capital", cabalport.StatusActive)
	f.card(mia, "mia", "Mia")
	f.card(kai, "kai", "")
	proposals := []ids.ProposalID{ids.ProposalIDFrom(closed.ID), ids.ProposalIDFrom(untouched.ID)}
	got, err := f.pendingReads().ProposalCards(t.Context(), ids.UserIDFrom(f.caller), proposals)
	if err != nil || len(got) != 2 {
		t.Fatalf("ProposalCards = %+v, %v, want two cards", got, err)
	}
	return closedCards{closed: got[proposals[0]], untouched: got[proposals[1]], decider: kai}
}

func TestProposalCards_ClosedCardCarriesTheTallyAndTheViewersBallot(t *testing.T) {
	t.Parallel()
	c := readClosedCards(t).closed
	if c.Status != domain.StatusPassed || c.YourChoice != "yes" {
		t.Errorf("closed card status and ballot = %s and %q, want passed and yes", c.Status, c.YourChoice)
	}
	if c.Yes != 3 || c.No != 1 || c.Voted != 4 || c.Eligible != 4 || c.ReplyCount != 0 {
		t.Errorf("closed card tally = %+v, want 3 yes, 1 no, 4 of 4 voted and no replies read", c)
	}
}

func TestProposalCards_ClosedCardCarriesTheFactsTheRowDraws(t *testing.T) {
	t.Parallel()
	c := readClosedCards(t).closed
	if c.CabalName != "Degens" || c.Symbol != "AAPLx" || c.AssetName != "Apple" ||
		c.AssetLogo != marketfake.AAPLx().LogoURL || c.Proposer.Name != "Mia" {
		t.Errorf("closed card facts = %+v", c)
	}
	if c.USDCMicros == nil || *c.USDCMicros != 400_000_000 || c.TokenAmount != nil {
		t.Errorf("closed card amounts = %v and %v, want a 400 USDC buy", c.USDCMicros, c.TokenAmount)
	}
}

func TestProposalCards_ReturnsTheDecidingVoter(t *testing.T) {
	t.Parallel()
	got := readClosedCards(t)
	if got.closed.DecidedBy == nil || got.closed.DecidedBy.UUID() != got.decider {
		t.Errorf("closed card DecidedBy = %v, want %s", got.closed.DecidedBy, got.decider)
	}
	if got.untouched.DecidedBy != nil {
		t.Errorf("open card DecidedBy = %v, want none", got.untouched.DecidedBy)
	}
}

func TestProposalCards_OpenCardWithoutABallotFallsBackToTheHandle(t *testing.T) {
	t.Parallel()
	u := readClosedCards(t).untouched
	if u.Status != domain.StatusOpen || u.YourChoice != "" || u.Voted != 0 || u.Eligible != 2 {
		t.Errorf("open card = %+v, want open with no ballot and two eligible", u)
	}
	if u.Proposer.Name != "kai" {
		t.Errorf("open card proposer = %q, want the handle when there is no display name", u.Proposer.Name)
	}
}

func TestProposalCards_OmitsUnknownIDsAndReadsEachPortOnce(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	proposer := f.ids.NewV7()
	a := f.open(t, f.ids.NewV7(), proposer, time.Hour, f.caller)
	b := f.open(t, f.ids.NewV7(), proposer, time.Hour, f.caller)
	f.insert(t, a)
	f.insert(t, b)
	proposals := []ids.ProposalID{
		ids.ProposalIDFrom(a.ID), ids.ProposalIDFrom(b.ID), ids.ProposalIDFrom(f.ids.NewV7()),
	}
	got, err := f.pendingReads().ProposalCards(t.Context(), ids.UserIDFrom(f.caller), proposals)
	if err != nil || len(got) != 2 {
		t.Fatalf("ProposalCards = %+v, %v, want the two known cards", got, err)
	}
	if _, ok := got[proposals[2]]; ok {
		t.Errorf("ProposalCards returned the unknown id")
	}
	if len(f.names.calls) != 1 || len(f.mints.calls) != 1 || len(f.cards.calls) != 1 || len(f.replies.calls) != 0 {
		t.Errorf("port reads = %d cabal, %d asset, %d user, %d reply, want 1, 1, 1, 0",
			len(f.names.calls), len(f.mints.calls), len(f.cards.calls), len(f.replies.calls))
	}
	empty, err := f.pendingReads().ProposalCards(t.Context(), ids.UserIDFrom(f.caller), nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("ProposalCards with no ids = %+v, %v, want none", empty, err)
	}
}

func TestProposalCards_FailuresAndEmptyReads(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	known := f.open(t, f.ids.NewV7(), f.ids.NewV7(), time.Hour, f.caller)
	f.insert(t, known)
	badKind := f.open(t, f.ids.NewV7(), f.ids.NewV7(), time.Hour, f.caller)
	badKind.Kind = "bogus"
	f.insert(t, badKind)
	viewer := ids.UserIDFrom(f.caller)
	ask := func(p ...sqlc.InsertProposalParams) []ids.ProposalID {
		out := make([]ids.ProposalID, len(p))
		for i, one := range p {
			out[i] = ids.ProposalIDFrom(one.ID)
		}
		return out
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.pendingReads().ProposalCards(cancelled, viewer, ask(known)); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ProposalCards on a cancelled context err = %v, want internal", err)
	}
	none, err := f.pendingReads().
		ProposalCards(t.Context(), viewer, []ids.ProposalID{ids.ProposalIDFrom(f.ids.NewV7())})
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("ProposalCards of an unknown id = %#v, %v, want an empty map", none, err)
	}
	if _, err := f.pendingReads().ProposalCards(t.Context(), viewer, ask(badKind)); err == nil {
		t.Error("ProposalCards of a row with an unknown kind err = nil, want one")
	}

	f.names.err = errs.New(errs.CodeDBUnavailable, "t")
	if _, err := f.pendingReads().ProposalCards(t.Context(), viewer, ask(known)); errs.CodeOf(
		err) != errs.CodeDBUnavailable {
		t.Errorf("ProposalCards with the cabal read down err = %v, want db_unavailable", err)
	}
}
