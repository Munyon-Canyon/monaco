package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (d voteDB) banVoids(swaps *fakes.Trading) adapters.BanVoids {
	return adapters.BanVoids{Voids: app.NewVoidProposalHandler(d.uow, d.ids, d.clk, swaps, app.NoHints{})}
}

func (d voteDB) ban(t *testing.T, h adapters.BanVoids, user uuid.UUID, to string) error {
	t.Helper()
	e := events.UserAccountStatusChanged{V: 1, UserID: user, From: "active", To: to, Cause: "admin"}
	return d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, e, d.now)
	})
}

func (d voteDB) proposedBy(t *testing.T, proposer uuid.UUID, status string) uuid.UUID {
	t.Helper()
	p := d.buy(d.ids.NewV7())
	p.ProposerID = proposer
	d.insert(t, p)
	if status != "open" {
		set := `UPDATE proposals SET status = $2 WHERE id = $1`
		if _, err := d.pool.Exec(t.Context(), set, p.ID, status); err != nil {
			t.Fatal(err)
		}
	}
	return p.ID
}

func TestAdminBanUser_VoidsOpenProposals(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	banned, other := d.ids.NewV7(), d.ids.NewV7()
	open1, open2 := d.proposedBy(t, banned, "open"), d.proposedBy(t, banned, "open")
	live := d.proposedBy(t, banned, "passed")
	executed := d.proposedBy(t, banned, "executed")
	theirs := d.proposedBy(t, other, "open")
	swaps := fakes.NewTrading(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: live}, Status: "submitted",
	})
	h := d.banVoids(swaps)
	for range 2 {
		if err := d.ban(t, h, banned, "banned"); err != nil {
			t.Fatalf("ban event = %v, want nil", err)
		}
	}
	for proposal, want := range map[uuid.UUID]string{
		open1: "voided", open2: "voided", live: "passed", executed: "executed", theirs: "open",
	} {
		if got := d.row(t, proposal).status.String; got != want {
			t.Errorf("proposal %s is %s, want %s", proposal, got, want)
		}
	}
	if r := d.row(t, open1); r.voidReason.String != "proposer banned" {
		t.Errorf("void_reason = %q, want proposer banned", r.voidReason.String)
	}
	for _, proposal := range []uuid.UUID{open1, open2} {
		if n := len(d.payloads(t, proposal, events.TypeProposalVoided)); n != 1 {
			t.Errorf("%d proposal.voided events for %s, want 1", n, proposal)
		}
		if n := len(d.auditOf(t, proposal)); n != 0 {
			t.Errorf("%d admin.action events for a system void, want 0", n)
		}
	}
}

func TestAdminBanUser_IgnoresOtherStatusChanges(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	user := d.ids.NewV7()
	open := d.proposedBy(t, user, "open")
	if err := d.ban(t, d.banVoids(fakes.NewTrading()), user, "suspended"); err != nil {
		t.Fatal(err)
	}
	if got := d.row(t, open).status.String; got != "open" {
		t.Fatalf("proposal is %s after a suspension, want open", got)
	}
}

func TestAdminBanUser_SkipsAProposalClosedMeanwhile(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	user := d.ids.NewV7()
	open := d.proposedBy(t, user, "open")
	racing := voidRacer{Voider: d.banVoids(fakes.NewTrading()).Voids, d: d}
	h := adapters.BanVoids{Voids: racing}
	if err := d.ban(t, h, user, "banned"); err != nil {
		t.Fatalf("ban event with a proposal closed meanwhile = %v, want nil", err)
	}
	if got := d.row(t, open).status.String; got != "withdrawn" {
		t.Fatalf("proposal is %s, want the concurrent withdrawal kept", got)
	}
}

type voidRacer struct {
	adapters.Voider
	d voteDB
}

func (r voidRacer) Within(ctx context.Context, tx db.Tx, cmd app.VoidProposal) error {
	_, err := tx.Queries().Exec(ctx, `UPDATE proposals SET status = 'withdrawn' WHERE id = $1`, cmd.ProposalID.UUID())
	if err != nil {
		return err
	}
	return r.Voider.Within(ctx, tx, cmd)
}

func TestAdminBanUser_ReturnsWhatItCannotSkip(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	user := d.ids.NewV7()
	d.proposedBy(t, user, "open")
	swaps := fakes.NewTrading()
	swaps.FailOnce(errs.New(errs.CodeDBUnavailable, "t"))
	if err := d.ban(t, d.banVoids(swaps), user, "banned"); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("ban event with a failing swap read = %v, want %s", err, errs.CodeDBUnavailable)
	}
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE proposals RENAME COLUMN proposer_id TO proposer`); err != nil {
		t.Fatal(err)
	}
	if err := d.ban(t, d.banVoids(swaps), user, "banned"); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ban event with an unreadable table = %v, want %s", err, errs.CodeInternal)
	}
}

func TestAdminBanUser_VoidsOnlyForAnAdminOrSystemActor(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	open := d.proposedBy(t, d.ids.NewV7(), "open")
	voids := app.NewVoidProposalHandler(d.uow, d.ids, d.clk, fakes.NewTrading(), app.NoHints{})
	err := d.uow.Do(as(t, auth.ActorUser), func(ctx context.Context, tx db.Tx) error {
		return voids.Within(ctx, tx, app.VoidProposal{ProposalID: ids.ProposalIDFrom(open), Reason: "x"})
	})
	if errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("Within as a user = %v, want %s", err, errs.CodeForbidden)
	}
}
