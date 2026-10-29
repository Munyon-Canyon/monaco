package governance_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"

	cabal "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type sideRoom struct {
	readsDB
	mia, kai, jordan uuid.UUID
	proposal         uuid.UUID
}

func newSideRoom(t *testing.T, rule bool, status domain.Status) sideRoom {
	t.Helper()
	d := newReadsDB(t)
	r := sideRoom{readsDB: d, mia: d.ids.NewV7(), kai: d.ids.NewV7(), jordan: d.ids.NewV7()}
	r.proposal = d.proposedBy(t, d.caller, r.mia, r.kai, r.jordan)
	d.ballot(t, r.proposal, d.caller, "yes")
	d.ballot(t, r.proposal, r.mia, "yes")
	d.ballot(t, r.proposal, r.kai, "no")
	if _, err := d.pool.Exec(
		t.Context(), `UPDATE proposals SET sides_after_close = $2, status = $3 WHERE id = $1`,
		r.proposal, rule, string(status),
	); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r sideRoom) voters(t *testing.T, caller uuid.UUID) (map[uuid.UUID]app.Voter, app.ProposalDetail) {
	t.Helper()
	got := r.get(t, r.proposal, caller)
	byID := map[uuid.UUID]app.Voter{}
	for _, v := range got.Voters {
		byID[v.UserID.UUID()] = v
	}
	return byID, got
}

func TestGetProposal_sidesPrivateWhileOpen(t *testing.T) {
	t.Parallel()
	for _, rule := range []bool{false, true} {
		t.Run(fmt.Sprintf("rule %v", rule), func(t *testing.T) {
			t.Parallel()
			checkOpenSidesPrivate(t, rule)
		})
	}
}

func checkOpenSidesPrivate(t *testing.T, rule bool) {
	t.Helper()
	r := newSideRoom(t, rule, domain.StatusOpen)
	byID, got := r.voters(t, r.caller)
	for _, other := range []uuid.UUID{r.mia, r.kai} {
		if v := byID[other]; !v.Voted || v.Choice != "" || !v.CastAt.IsZero() {
			t.Errorf("rule %v: other voter = %+v, want voted with no side", rule, v)
		}
	}
	if v := byID[r.caller]; !v.Voted || v.Choice != domain.ChoiceYes || v.CastAt.IsZero() {
		t.Errorf("rule %v: caller = %+v, want their own side", rule, v)
	}
	if v := byID[r.jordan]; v.Voted {
		t.Errorf("rule %v: jordan = %+v, want not voted", rule, v)
	}
	if got.Proposal.Tally.Yes != 2 || got.Proposal.Tally.No != 1 || got.SidesVisible {
		t.Errorf("rule %v: tally = %+v visible = %v, want live counts and private sides",
			rule, got.Proposal.Tally, got.SidesVisible)
	}
}

func TestGetProposal_sidesRevealedAfterCloseWhenRuleOn(t *testing.T) {
	t.Parallel()
	for _, status := range []domain.Status{
		domain.StatusPassed, domain.StatusFailed, domain.StatusExpired, domain.StatusExecuted,
		domain.StatusExecutionBlocked,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			checkClosedSidesRevealed(t, status)
		})
	}
}

func checkClosedSidesRevealed(t *testing.T, status domain.Status) {
	t.Helper()
	r := newSideRoom(t, true, status)
	byID, got := r.voters(t, r.caller)
	if !got.SidesVisible || byID[r.mia].Choice != domain.ChoiceYes || byID[r.kai].Choice != domain.ChoiceNo ||
		byID[r.mia].CastAt.IsZero() || byID[r.kai].CastAt.IsZero() {
		t.Errorf("%s: voters = %+v visible = %v, want every side", status, byID, got.SidesVisible)
	}
	if v := byID[r.jordan]; v.Voted || v.Choice != "" || !v.CastAt.IsZero() {
		t.Errorf("%s: jordan = %+v, want not voted and no side", status, v)
	}
}

func TestGetProposal_ruleTurnedOnLaterDoesNotReveal(t *testing.T) {
	t.Parallel()
	r := newSideRoom(t, false, domain.StatusPassed)
	byID, got := r.voters(t, r.caller)
	if got.SidesVisible || byID[r.mia].Choice != "" || byID[r.kai].Choice != "" {
		t.Fatalf(
			"voters = %+v visible = %v, want a proposal opened under privacy to stay private",
			byID,
			got.SidesVisible,
		)
	}
}

func TestGetProposal_withdrawnNeverReveals(t *testing.T) {
	t.Parallel()
	for _, status := range []domain.Status{domain.StatusWithdrawn, domain.StatusVoided} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			r := newSideRoom(t, true, status)
			byID, got := r.voters(t, r.caller)
			if got.SidesVisible || byID[r.mia].Choice != "" || byID[r.kai].Choice != "" || !byID[r.mia].Voted {
				t.Errorf("%s: voters = %+v visible = %v, want no sides", status, byID, got.SidesVisible)
			}
		})
	}
}

func anySide(byID map[uuid.UUID]app.Voter, who ...uuid.UUID) bool {
	for _, id := range who {
		if byID[id].Choice != "" || !byID[id].CastAt.IsZero() {
			return true
		}
	}
	return false
}

func TestGetProposal_nonMemberSeesNoSides(t *testing.T) {
	t.Parallel()
	t.Run("open", func(t *testing.T) {
		t.Parallel()
		r := newSideRoom(t, true, domain.StatusOpen)
		byID, got := r.voters(t, r.ids.NewV7())
		if got.SidesVisible || anySide(byID, r.mia, r.kai, r.caller) || !byID[r.mia].Voted {
			t.Errorf("voters = %+v visible = %v, want counts only", byID, got.SidesVisible)
		}
	})
	t.Run("closed with the rule off", func(t *testing.T) {
		t.Parallel()
		r := newSideRoom(t, false, domain.StatusPassed)
		byID, got := r.voters(t, r.ids.NewV7())
		if got.SidesVisible || anySide(byID, r.mia, r.kai, r.caller) {
			t.Errorf("voters = %+v visible = %v, want no choices", byID, got.SidesVisible)
		}
	})
	t.Run("closed with the rule on", func(t *testing.T) {
		t.Parallel()
		r := newSideRoom(t, true, domain.StatusPassed)
		member, _ := r.voters(t, r.mia)
		nonMember, got := r.voters(t, r.ids.NewV7())
		if !reflect.DeepEqual(member, nonMember) || !got.SidesVisible || nonMember[r.mia].Choice != domain.ChoiceYes {
			t.Errorf("non-member sees %+v, member sees %+v, want the same revealed sides", nonMember, member)
		}
	})
}

func TestGetProposal_votersCarryNameAndPhoto(t *testing.T) {
	t.Parallel()
	r := newSideRoom(t, false, domain.StatusOpen)
	book := &cardBook{cards: map[ids.UserID]identityport.UserCard{
		ids.UserIDFrom(r.caller): {DisplayName: "Andres"},
		ids.UserIDFrom(r.mia):    {DisplayName: "Mia", PhotoURL: "https://example.com/mia.png"},
		ids.UserIDFrom(r.kai):    {DisplayName: "Kai", Deleted: true},
	}}
	reads := app.NewProposalReads(r.pool, r.swaps, r.names, r.assets, book, &returnBook{}, &replyBook{})
	got, err := reads.Get(
		t.Context(),
		app.GetProposal{ID: ids.ProposalIDFrom(r.proposal), Caller: ids.UserIDFrom(r.caller)},
	)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[uuid.UUID]app.Voter{}
	for _, v := range got.Voters {
		byID[v.UserID.UUID()] = v
	}
	if v := byID[r.mia]; v.DisplayName != "Mia" || v.PhotoURL != "https://example.com/mia.png" {
		t.Errorf("mia = %+v, want her name and photo", v)
	}
	if v := byID[r.caller]; v.DisplayName != "Andres" || v.PhotoURL != "" {
		t.Errorf("caller = %+v, want a name and no photo", v)
	}
	for _, missing := range []uuid.UUID{r.kai, r.jordan} {
		if v := byID[missing]; v.DisplayName != "" || v.PhotoURL != "" {
			t.Errorf("voter without a live card = %+v, want an empty name", v)
		}
	}
	if len(book.calls) != 1 || len(book.calls[0]) != 4 {
		t.Errorf("identity reads = %v, want one batched read of the four voters", book.calls)
	}
}

type showSidesCabals struct{ app.Cabals }

func (c showSidesCabals) Rules(ctx context.Context, id ids.CabalID) (cabal.Rules, error) {
	rules, err := c.Cabals.Rules(ctx, id)
	rules.ShowSides = true
	return rules, err
}

func TestOpenProposal_snapshotsSidesRule(t *testing.T) {
	t.Parallel()
	for name, show := range map[string]bool{"rule on": true, "rule off": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkSidesSnapshot(t, show)
		})
	}
}

func checkSidesSnapshot(t *testing.T, show bool) {
	t.Helper()
	name := "propose"
	h := newProposeHarness(t)
	p := h.w.ports()
	if show {
		p.Cabals = showSidesCabals{p.Cabals}
	}
	handler := app.NewProposeTradeHandler(h.guard, h.d.ids, clock.Real{}, p, app.NoHints{})
	ctx := observability.WithActor(t.Context(), "user:"+h.w.members[0].String())
	id, err := handler.Handle(ctx, app.ProposeTrade{
		CabalID: h.w.cabal, ProposerID: h.w.members[0], Trade: buyAAPLFor(potMicros),
	})
	if err != nil {
		t.Fatalf("%s: propose = %v", name, err)
	}
	var got bool
	err = h.d.pool.QueryRow(t.Context(), `SELECT sides_after_close FROM proposals WHERE id = $1`, id.UUID()).
		Scan(&got)
	if err != nil || got != show {
		t.Errorf("%s: sides_after_close = %v, %v, want %v", name, got, err, show)
	}
}
