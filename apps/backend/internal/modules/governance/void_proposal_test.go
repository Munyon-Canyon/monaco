package governance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (d voteDB) void(ctx context.Context, swaps *fakes.Trading, proposal uuid.UUID) error {
	return app.NewVoidProposalHandler(d.uow, d.pool, d.clk, swaps).Handle(ctx, app.VoidProposal{
		ProposalID: ids.ProposalIDFrom(proposal), Reason: "spam",
	})
}

func as(t *testing.T, kind auth.ActorKind) context.Context {
	t.Helper()
	if kind == "" {
		return t.Context()
	}
	return auth.WithActor(t.Context(), auth.Actor{Kind: kind, ID: "ops-1"})
}

func TestVoidProposal_refusesWhatItMustNotVoid(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	open, _ := d.open(t, 2)
	failed, _ := d.open(t, 1)
	_, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = 'failed' WHERE id = $1`, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	traded, _ := d.open(t, 1)
	swaps := fakes.NewTrading(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: traded.ID}, Status: "submitted",
	})
	for name, tc := range map[string]struct {
		actor    auth.ActorKind
		proposal uuid.UUID
		want     errs.Code
	}{
		"no actor":         {"", open.ID, errs.CodeUnauthorized},
		"a user":           {auth.ActorUser, open.ID, errs.CodeForbidden},
		"an agent":         {auth.ActorAgent, open.ID, errs.CodeForbidden},
		"unknown":          {auth.ActorAdmin, d.ids.NewV7(), errs.CodeProposalNotFound},
		"failed":           {auth.ActorAdmin, failed.ID, errs.CodeProposalClosed},
		"a submitted swap": {auth.ActorSystem, traded.ID, errs.CodeLiveSwapExists},
	} {
		if err := d.void(as(t, tc.actor), swaps, tc.proposal); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: VoidProposal = %v, want %s", name, err, tc.want)
		}
	}
	swaps.FailOnce(errs.New(errs.CodeDBUnavailable, "t"))
	if err := d.void(as(t, auth.ActorAdmin), swaps, open.ID); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("VoidProposal whose swap read fails = %v, want db_unavailable", err)
	}
	_, err = d.pool.Exec(t.Context(),
		`ALTER TABLE proposals ADD CONSTRAINT no_void CHECK (status <> 'voided') NOT VALID`)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.void(as(t, auth.ActorAdmin), swaps, open.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("VoidProposal whose update fails = %v, want internal", err)
	}
	for _, p := range []uuid.UUID{open.ID, failed.ID, traded.ID} {
		if n := len(d.payloads(t, p, events.TypeProposalVoided)); n != 0 {
			t.Errorf("%s has %d proposal.voided events after refusals, want 0", p, n)
		}
	}
	if r := d.row(t, open.ID); r.status.String != "open" {
		t.Errorf("refused proposal is %s, want open", r.status.String)
	}
}

func TestVoidProposal_anAdminVoidsAnOpenProposalWithItsReason(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.open(t, 2)
	if err := d.void(as(t, auth.ActorAdmin), fakes.NewTrading(), p.ID); err != nil {
		t.Fatalf("VoidProposal = %v, want nil", err)
	}
	var status, reason string
	if err := d.pool.QueryRow(t.Context(), `SELECT status, void_reason FROM proposals WHERE id = $1`, p.ID).
		Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "voided" || reason != "spam" {
		t.Errorf("row = %s %q, want voided \"spam\"", status, reason)
	}
	got := d.payloads(t, p.ID, events.TypeProposalVoided)
	if len(got) != 1 || !strings.Contains(string(got[0]), `"actor_type": "admin"`) {
		t.Fatalf("proposal.voided payloads = %s, want one with actor_type admin", got)
	}
}

func TestParseVoidReason_takesOneTo500Characters(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
		"":                       false,
		"   ":                    false,
		"x":                      true,
		strings.Repeat("é", 500): true,
		strings.Repeat("é", 501): false,
	} {
		_, err := domain.ParseVoidReason(raw)
		if (err == nil) != ok || err != nil && errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseVoidReason(%d runes) = %v, want ok %t", len([]rune(raw)), err, ok)
		}
	}
}
