package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (d voteDB) voidAs(ctx context.Context, proposal uuid.UUID, reason string) error {
	return app.NewVoidProposalHandler(d.uow, d.pool, d.ids, d.clk, fakes.NewTrading(), app.NoHints{}).
		Handle(ctx, app.VoidProposal{ProposalID: ids.ProposalIDFrom(proposal), Reason: domain.VoidReason(reason)})
}

func (d voteDB) auditOf(t *testing.T, proposal uuid.UUID) []events.AdminAction {
	t.Helper()
	rows, err := d.pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = $1 AND payload->>'target_id' = $2 ORDER BY id`,
		string(events.TypeAdminAction), proposal.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.AdminAction
	for rows.Next() {
		var raw []byte
		var action events.AdminAction
		if err := rows.Scan(&raw); err != nil || json.Unmarshal(raw, &action) != nil {
			t.Fatalf("scan admin.action: %v", err)
		}
		out = append(out, action)
	}
	return out
}

func statusOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got.Status
}

func TestVoidProposal_AnAdminVoidAppendsTheAuditInTheSameTransaction(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	open, _ := d.open(t, 2)
	passed, _ := d.open(t, 1)
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE proposals SET status = 'passed' WHERE id = $1`,
		passed.ID,
	); err != nil {
		t.Fatal(err)
	}
	for proposal, from := range map[uuid.UUID]string{open.ID: "open", passed.ID: "passed"} {
		if err := d.voidAs(as(t, auth.ActorAdmin), proposal, "spam proposal"); err != nil {
			t.Fatalf("VoidProposal(%s) = %v, want nil", from, err)
		}
		got := d.auditOf(t, proposal)
		if len(got) != 1 {
			t.Fatalf("%s: %d admin.action events, want 1", from, len(got))
		}
		a := got[0]
		if a.Action != events.AdminActionProposalVoid || a.TargetType != events.AdminTargetProposal ||
			a.AdminID.String() != opsID || a.Reason != "spam proposal" ||
			statusOf(t, a.Before) != from || statusOf(t, a.After) != "voided" {
			t.Errorf("%s: admin.action = %+v, want proposal_void by the admin from %s to voided", from, a, from)
		}
		if n := len(d.payloads(t, proposal, events.TypeProposalVoided)); n != 1 {
			t.Errorf("%s: %d proposal.voided events, want 1", from, n)
		}
	}
}

func TestAdminVoid_SystemActor_NoAdminAction(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.open(t, 1)
	if err := d.voidAs(as(t, auth.ActorSystem), p.ID, "x"); err != nil {
		t.Fatalf("VoidProposal as system = %v, want nil", err)
	}
	if n := len(d.auditOf(t, p.ID)); n != 0 {
		t.Errorf("%d admin.action events after a system void, want 0", n)
	}
	if n := len(d.payloads(t, p.ID, events.TypeProposalVoided)); n != 1 {
		t.Errorf("%d proposal.voided events, want 1", n)
	}
}

func TestVoidProposal_AnAdminNeedsAUserIDAndAReasonOfThreeCharacters(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.open(t, 1)
	for name, tc := range map[string]struct {
		actor  string
		reason string
		want   errs.Code
	}{
		"a service id":     {"ops-1", "spam proposal", errs.CodeAdminForbidden},
		"two characters":   {opsID, "ab", errs.CodeReasonRequired},
		"only whitespace":  {opsID, "   ", errs.CodeReasonRequired},
		"a bare character": {opsID, "x", errs.CodeReasonRequired},
	} {
		ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: tc.actor})
		if err := d.voidAs(ctx, p.ID, tc.reason); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: VoidProposal = %v, want %s", name, err, tc.want)
		}
	}
	if r := d.row(t, p.ID); r.status.String != "open" {
		t.Errorf("refused proposal is %s, want open", r.status.String)
	}
	if n := len(d.auditOf(t, p.ID)) + len(d.payloads(t, p.ID, events.TypeProposalVoided)); n != 0 {
		t.Errorf("%d events after refusals, want 0", n)
	}
}

func TestVoidProposal_RollsBackWhenTheAuditCannotBeAppended(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.open(t, 1)
	_, err := d.pool.Exec(t.Context(),
		`ALTER TABLE events ADD CONSTRAINT no_audit CHECK (type <> 'admin.action') NOT VALID`)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.voidAs(as(t, auth.ActorAdmin), p.ID, "spam proposal"); err == nil {
		t.Fatal("VoidProposal = nil with the audit rejected, want an error")
	}
	if r := d.row(t, p.ID); r.status.String != "open" {
		t.Errorf("proposal is %s after the failed audit, want open", r.status.String)
	}
	if n := len(d.payloads(t, p.ID, events.TypeProposalVoided)); n != 0 {
		t.Errorf("%d proposal.voided events after the failed audit, want 0", n)
	}
}

func seedRoutedProposal(t *testing.T, s *scenario.Scenario) string {
	t.Helper()
	c := testkit.NewCabal(t, s.DB(), testkit.WithMembers(2))
	id, now := testkit.NewIDs(21).NewV7(), clock.Real{}.Now().UTC()
	params := sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true}, QuoteOutAmount: 21_000_000,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	}
	for _, m := range c.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	if _, err := sqlc.New(s.DB()).InsertProposal(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestAdminVoid_ModeratorOk_ViewerForbidden(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	id := seedRoutedProposal(t, s)
	path := "/v1/admin/proposals/" + id + "/void"
	s.Given(scenario.SeededAdmin("viewer", "viewer"), scenario.SeededAdmin("mod", "moderator")).
		When(
			scenario.AsUser("viewer"),
			scenario.Post(path, `{"reason":"spam proposal"}`),
			scenario.ExpectStatus(http.StatusForbidden),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
			scenario.AsUser("mod"),
			scenario.Post(path, `{"reason":"ab"}`),
			scenario.ExpectStatus(http.StatusBadRequest),
			scenario.ExpectProblem(errs.CodeReasonRequired),
			scenario.Post("/v1/admin/proposals/"+uuid.NewString()+"/void", `{"reason":"spam proposal"}`),
			scenario.ExpectStatus(http.StatusNotFound),
			scenario.ExpectProblem(errs.CodeProposalNotFound),
			scenario.Post(path, `{"reason":"  spam proposal  "}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("id", id),
			scenario.ExpectJSON("status", "voided"),
			scenario.ExpectJSON("void_reason", "spam proposal"),
			scenario.Replay(),
			scenario.Post(path, `{"reason":"spam proposal"}`),
			scenario.ExpectStatus(http.StatusUnprocessableEntity),
			scenario.ExpectProblem(errs.CodeProposalClosed),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalVoided, 1),
			scenario.ExpectAdminAction(events.AdminActionProposalVoid, id),
		)
}
