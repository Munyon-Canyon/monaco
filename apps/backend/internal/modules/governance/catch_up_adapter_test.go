package governance_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestCatchUp_PendingVotes_MapsEachVoteOntoTheCard(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	mia := f.ids.NewV7()
	p := f.open(t, f.ids.NewV7(), mia, time.Hour, f.caller, mia)
	f.insert(t, p)
	f.ballot(t, p.ID, mia, "yes")
	f.names.set(p.CabalID, "AC Capital", cabalport.StatusActive)
	f.card(mia, "mia", "")
	f.replies.counts[ids.ProposalIDFrom(p.ID)] = 3

	got, err := adapters.NewCatchUp(f.pendingReads()).PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if err != nil || len(got) != 1 {
		t.Fatalf("PendingVotes = %+v, %v, want one card", got, err)
	}
	c := got[0]
	if c.USDCMicros == nil || *c.USDCMicros != 400_000_000 {
		t.Errorf("USDC = %v, want 400000000", c.USDCMicros)
	}
	c.USDCMicros, c.CreatedAt, c.ExpiresAt = nil, time.Time{}, time.Time{}
	want := port.CatchUpProposal{
		ID: ids.ProposalIDFrom(p.ID), CabalID: ids.CabalIDFrom(p.CabalID), CabalName: "AC Capital", Kind: "buy",
		Symbol: "AAPLx", AssetName: "Apple", AssetLogo: marketfake.AAPLx().LogoURL,
		Proposer: port.CatchUpPerson{ID: ids.UserIDFrom(mia), Name: "mia", PhotoURL: "https://cdn.example.com/mia"},
		Status:   domain.StatusOpen, Yes: 1, Voted: 1, Eligible: 2, ReplyCount: 3,
	}
	if c != want {
		t.Errorf("card = %+v, want %+v", c, want)
	}
}

func TestCatchUp_PendingVotes_ReturnsTheReadError(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	f.insert(t, f.open(t, f.ids.NewV7(), f.ids.NewV7(), time.Hour, f.caller))
	f.replies.err = errs.New(errs.CodeDBUnavailable, "t")
	_, err := adapters.NewCatchUp(f.pendingReads()).PendingVotes(t.Context(), ids.UserIDFrom(f.caller))
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("PendingVotes err = %v, want db_unavailable", err)
	}
}

func TestCatchUp_ProposalCards_PassesTheReadThrough(t *testing.T) {
	t.Parallel()
	f := newPendingFacts(t)
	p := f.open(t, f.ids.NewV7(), f.ids.NewV7(), time.Hour, f.caller)
	f.insert(t, p)
	id := ids.ProposalIDFrom(p.ID)
	got, err := adapters.NewCatchUp(f.pendingReads()).ProposalCards(
		t.Context(), ids.UserIDFrom(f.caller), []ids.ProposalID{id},
	)
	if err != nil || len(got) != 1 || got[id].ID != id {
		t.Fatalf("ProposalCards = %+v, %v, want the one card", got, err)
	}
}
