package governance_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (d proposalDB) facts() app.Queries {
	return governance.New(module.Deps{Pool: d.pool}).InviteFacts()
}

func (d proposalDB) proposal(
	t *testing.T, cabal uuid.UUID, createdAt time.Time, voters ...uuid.UUID,
) uuid.UUID {
	t.Helper()
	p := d.buy(voters...)
	p.CabalID, p.CreatedAt = cabal, createdAt
	d.insert(t, p)
	return p.ID
}

func TestInviteFacts_NewestOpenProposal_ReturnsTheNewestOpenOneWithItsTally(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	cabal := d.ids.NewV7()
	voters := []uuid.UUID{d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()}
	older := d.proposal(t, cabal, d.now.Add(-2*time.Hour), voters...)
	newer := d.buy(voters...)
	newer.CabalID, newer.CreatedAt = cabal, d.now.Add(-time.Hour)
	newer.Thesis = pgtype.Text{String: "small add\nbefore earnings", Valid: true}
	d.insert(t, newer)
	d.ballot(t, newer.ID, voters[0], "yes")
	d.ballot(t, newer.ID, voters[1], "yes")
	d.ballot(t, newer.ID, voters[2], "no")
	d.ballot(t, older, voters[3], "yes")

	got, ok, err := d.facts().NewestOpenProposal(t.Context(), ids.CabalIDFrom(cabal))
	if err != nil || !ok {
		t.Fatalf("NewestOpenProposal = %+v, %v, %v, want the newer proposal", got, ok, err)
	}
	usdc := uint64(25_000_000)
	want := port.OpenProposal{
		ID: ids.ProposalIDFrom(newer.ID), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint, USDCMicros: &usdc,
		ProposerID: ids.UserIDFrom(newer.ProposerID), Thesis: "small add\nbefore earnings", Yes: 2, No: 1, Voted: 3,
		Eligible: 4, ExpiresAt: newer.ExpiresAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("proposal = %+v, want %+v", got, want)
	}
}

func TestInviteFacts_NewestOpenProposal_SkipsAClosedOneAndOtherCabals(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	cabal, other := d.ids.NewV7(), d.ids.NewV7()
	voter := d.ids.NewV7()
	open := d.proposal(t, cabal, d.now.Add(-3*time.Hour), voter)
	closed := d.proposal(t, cabal, d.now.Add(-time.Hour), voter)
	d.proposal(t, other, d.now, voter)
	if n := d.transition(t, closed, "open", "passed", "majority"); n != 1 {
		t.Fatalf("transition = %d rows, want 1", n)
	}
	got, ok, err := d.facts().NewestOpenProposal(t.Context(), ids.CabalIDFrom(cabal))
	if err != nil || !ok || got.ID.UUID() != open {
		t.Fatalf("NewestOpenProposal = %+v, %v, %v, want the older open proposal %s", got, ok, err, open)
	}
	_, ok, err = d.facts().NewestOpenProposal(t.Context(), ids.CabalIDFrom(d.ids.NewV7()))
	if err != nil || ok {
		t.Fatalf("NewestOpenProposal for a cabal with none = %v, %v, want false", ok, err)
	}
}

func TestInviteFacts_NewestOpenProposal_ASellHasNoUSDC(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	p := d.buy(d.ids.NewV7())
	p.Kind, p.UsdcMicros, p.TokenAmount = "sell", pgtype.Int8{}, pgtype.Int8{Int64: 150_000_000, Valid: true}
	d.insert(t, p)
	got, ok, err := d.facts().NewestOpenProposal(t.Context(), ids.CabalIDFrom(p.CabalID))
	if err != nil || !ok || got.Kind != "sell" || got.USDCMicros != nil {
		t.Fatalf("NewestOpenProposal = %+v, %v, %v, want a sell with no USDC", got, ok, err)
	}
}

func TestInviteFacts_MonthTurnout_CountsBallotsOverEligibleSince(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	cabal := d.ids.NewV7()
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	a, b, c := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	first := d.proposal(t, cabal, month.Add(time.Hour), a, b, c)
	second := d.proposal(t, cabal, month.Add(48*time.Hour), a, b)
	before := d.proposal(t, cabal, month.Add(-time.Hour), a, b, c)
	d.proposal(t, d.ids.NewV7(), month.Add(time.Hour), a, b, c)
	d.ballot(t, first, a, "yes")
	d.ballot(t, first, b, "no")
	d.ballot(t, second, a, "yes")
	d.ballot(t, before, a, "yes")
	d.ballot(t, before, b, "yes")

	voted, eligible, err := d.facts().MonthTurnout(t.Context(), ids.CabalIDFrom(cabal), month)
	if err != nil || voted != 3 || eligible != 5 {
		t.Fatalf("MonthTurnout = %d of %d, %v, want 3 of 5", voted, eligible, err)
	}
	voted, eligible, err = d.facts().MonthTurnout(t.Context(), ids.CabalIDFrom(cabal), month.Add(-2*time.Hour))
	if err != nil || voted != 5 || eligible != 8 {
		t.Fatalf("MonthTurnout from earlier = %d of %d, %v, want 5 of 8", voted, eligible, err)
	}
}

func TestInviteFacts_MonthTurnout_ACabalWithNoProposalsGivesZeroOverZero(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	voted, eligible, err := d.facts().MonthTurnout(t.Context(), ids.CabalIDFrom(d.ids.NewV7()), d.now)
	if err != nil || voted != 0 || eligible != 0 {
		t.Fatalf("MonthTurnout = %d of %d, %v, want 0 of 0", voted, eligible, err)
	}
}

func TestInviteFacts_FailWhenTheDatabaseCallFails(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cabal := ids.CabalIDFrom(d.ids.NewV7())
	if _, _, err := d.facts().NewestOpenProposal(ctx, cabal); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("NewestOpenProposal err = %v, want internal", err)
	}
	if _, _, err := d.facts().MonthTurnout(ctx, cabal, d.now); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("MonthTurnout err = %v, want internal", err)
	}
}
