package social_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f chatFixture) routes() adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(f.users), social.WithRealtime(f.rt)))
}

func (f chatFixture) removeAs(ctx context.Context, message uuid.UUID, reason string) error {
	_, err := f.routes().RemoveChatMessage(ctx, api.RemoveChatMessageRequestObject{
		Id: message, Body: &apibase.ReasonBody{Reason: reason},
	})
	return err
}

func (f chatFixture) adminActions(t *testing.T) []events.AdminAction {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeAdminAction))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.AdminAction
	for rows.Next() {
		var raw []byte
		var a events.AdminAction
		if err := rows.Scan(&raw); err != nil || json.Unmarshal(raw, &a) != nil {
			t.Fatalf("scan admin.action: %v", err)
		}
		out = append(out, a)
	}
	return out
}

func TestAdminRemoveChatMessage_softDeletesOnceAuditsAndTellsTheCabal(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	m := f.mustSend(t, f.member(1), "secret abuse", &domain.Reply{Parent: top.ID})
	before := len(f.rt.Published())
	for range 2 {
		if err := f.removeAs(asAdmin(t), m.ID, "abuse"); err != nil {
			t.Fatalf("RemoveChatMessage = %v, want nil", err)
		}
	}
	if got := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); got != 1 {
		t.Fatalf("%d deleted messages, want 1", got)
	}
	if n, _ := f.parentState(t, top.ID); n != 0 {
		t.Fatalf("parent after the removal = %d replies, want 0", n)
	}
	if got := len(f.rt.Published()) - before; got != 2 {
		t.Fatalf("%d realtime publishes, want 2: message deleted and thread updated, once", got)
	}
	f.expectChatAudit(t, m.ID)
}

func (f chatFixture) expectChatAudit(t *testing.T, message uuid.UUID) {
	t.Helper()
	audits := f.adminActions(t)
	if len(audits) != 1 {
		t.Fatalf("%d admin.action events, want 1", len(audits))
	}
	a := audits[0]
	if a.Action != events.AdminActionChatRemove || a.TargetType != events.AdminTargetChat ||
		a.TargetID != message.String() || a.AdminID != moderatorID() || a.Reason != "abuse" {
		t.Errorf("admin.action = %+v, want the moderator's chat_message_remove of the message", a)
	}
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "secret abuse") {
		t.Errorf("admin.action %s holds the message body", raw)
	}
}

func TestAdminRemoveChatMessage_Refusals(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "hello", nil)
	user := &auth.Actor{Kind: auth.ActorUser, ID: f.member(0).String()}
	admin := &auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID().String(), Role: "moderator"}
	service := &auth.Actor{Kind: auth.ActorAdmin, ID: "ops-1"}
	for name, tc := range map[string]struct {
		actor   *auth.Actor
		message uuid.UUID
		reason  string
		want    errs.Code
	}{
		"no actor":     {nil, m.ID, "abuse", errs.CodeUnauthorized},
		"a user":       {user, m.ID, "abuse", errs.CodeForbidden},
		"a service id": {service, m.ID, "abuse", errs.CodeAdminForbidden},
		"unknown":      {admin, f.deps.IDs.NewV7(), "abuse", errs.CodeChatMessageNotFound},
		"short reason": {admin, m.ID, "no", errs.CodeReasonRequired},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if err := f.removeAs(ctx, tc.message, tc.reason); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: RemoveChatMessage = %v, want %s", name, err, tc.want)
		}
	}
	if got := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); got != 0 {
		t.Errorf("%d deleted messages after refusals, want 0", got)
	}
	if n := len(f.adminActions(t)); n != 0 {
		t.Errorf("%d admin.action events after refusals, want 0", n)
	}
}

func (f reportFixture) resolveAs(ctx context.Context, report uuid.UUID, reason string) error {
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps, social.WithUsers(f.users)))
	_, err := routes.ResolveReport(ctx, api.ResolveReportRequestObject{
		Id: report, Body: &apibase.ReasonBody{Reason: reason},
	})
	return err
}

func (f reportFixture) status(t *testing.T, report uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM reports WHERE id = $1`, report).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAdminResolveReport_resolvesOnceAndAudits(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	id, err := f.file(t, f.reporter, domain.ReportMessage, f.message)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.resolveAs(asAdmin(t), id, "removed the message"); err != nil {
			t.Fatalf("ResolveReport = %v, want nil", err)
		}
	}
	if got := f.status(t, id); got != "resolved" {
		t.Fatalf("status = %q, want resolved", got)
	}
	var audits int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`,
		string(events.TypeAdminAction)).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("%d admin.action events (%v), want 1", audits, err)
	}
}

func TestAdminResolveReport_Refusals(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	id, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	if err != nil {
		t.Fatal(err)
	}
	user := &auth.Actor{Kind: auth.ActorUser, ID: f.reporter.String()}
	admin := &auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID().String(), Role: "moderator"}
	for name, tc := range map[string]struct {
		actor  *auth.Actor
		report uuid.UUID
		reason string
		want   errs.Code
	}{
		"a user":       {user, id, "done", errs.CodeForbidden},
		"unknown":      {admin, f.gen.NewV7(), "done", errs.CodeReportNotFound},
		"short reason": {admin, id, "no", errs.CodeReasonRequired},
	} {
		if err := f.resolveAs(
			auth.WithActor(t.Context(), *tc.actor),
			tc.report,
			tc.reason,
		); errs.CodeOf(
			err,
		) != tc.want {
			t.Errorf("%s: ResolveReport = %v, want %s", name, err, tc.want)
		}
	}
	if got := f.status(t, id); got != "open" {
		t.Errorf("status after refusals = %q, want open", got)
	}
}

func TestAdminChatAndReportCommands_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	reason, err := events.NewReason("abuse")
	if err != nil {
		t.Fatal(err)
	}
	admin := ids.UserIDFrom(moderatorID())
	for name, tc := range map[string]struct {
		breaker string
		run     func(t *testing.T, f chatFixture, top, reply uuid.UUID) error
	}{
		"remove when the lock fails": {
			`DROP TABLE cabal_messages`,
			func(t *testing.T, f chatFixture, top, _ uuid.UUID) error {
				t.Helper()
				return app.NewRemoveChatMessageHandler(f.deps).Handle(asAdmin(t),
					app.RemoveChatMessage{MessageID: top, Reason: reason, Admin: admin})
			},
		},
		"remove when the update fails": {
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_deletes CHECK (deleted_at IS NULL) NOT VALID`,
			func(t *testing.T, f chatFixture, top, _ uuid.UUID) error {
				t.Helper()
				return app.NewRemoveChatMessageHandler(f.deps).Handle(asAdmin(t),
					app.RemoveChatMessage{MessageID: top, Reason: reason, Admin: admin})
			},
		},
		"remove a reply when lowering the parent's count fails": {
			`ALTER TABLE cabal_messages ADD CONSTRAINT keep_replies CHECK (parent_id IS NOT NULL OR reply_count > 0) NOT VALID`,
			func(t *testing.T, f chatFixture, _, reply uuid.UUID) error {
				t.Helper()
				return app.NewRemoveChatMessageHandler(f.deps).Handle(asAdmin(t),
					app.RemoveChatMessage{MessageID: reply, Reason: reason, Admin: admin})
			},
		},
		"remove when the audit cannot be appended": {
			`DROP TABLE events CASCADE`,
			func(t *testing.T, f chatFixture, top, _ uuid.UUID) error {
				t.Helper()
				return app.NewRemoveChatMessageHandler(f.deps).Handle(asAdmin(t),
					app.RemoveChatMessage{MessageID: top, Reason: reason, Admin: admin})
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newChatFixture(t)
			top := f.mustSend(t, f.member(0), "top", nil)
			reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
			if _, err := f.pool.Exec(t.Context(), tc.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tc.run(t, f, top.ID, reply.ID), errs.CodeInternal)
		})
	}
}

func TestAdminResolveReport_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	reason, err := events.NewReason("handled")
	if err != nil {
		t.Fatal(err)
	}
	for name, breaker := range map[string]string{
		"the lock fails":               `DROP TABLE reports`,
		"the update fails":             `ALTER TABLE reports ADD CONSTRAINT stays_open CHECK (status = 'open') NOT VALID`,
		"the audit cannot be appended": `DROP TABLE events CASCADE`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReportFixture(t)
			id, err := f.file(t, f.reporter, domain.ReportMessage, f.message)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(t.Context(), breaker); err != nil {
				t.Fatal(err)
			}
			handler := app.NewResolveReportHandler(db.New(f.pool, f.gen, f.clock), f.gen, f.clock)
			cmd := app.ResolveReport{ReportID: id, Reason: reason, Admin: ids.UserIDFrom(moderatorID())}
			wantCode(t, handler.Handle(asAdmin(t), cmd), errs.CodeInternal)
		})
	}
}

func TestAdminRemoveChatMessage_refusesAMissingReasonBeforeCommitting(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "hello", nil)
	admin := ids.UserIDFrom(moderatorID())
	err := app.NewRemoveChatMessageHandler(f.deps).
		Handle(asAdmin(t), app.RemoveChatMessage{MessageID: m.ID, Admin: admin})
	wantCode(t, err, errs.CodeReasonRequired)
	if got := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); got != 0 {
		t.Fatalf("%d deleted messages after a refused removal, want 0", got)
	}
}

func TestAdminResolveReport_refusesAMissingReasonBeforeCommitting(t *testing.T) {
	t.Parallel()
	admin := ids.UserIDFrom(moderatorID())
	r := newReportFixture(t)
	id, err := r.file(t, r.reporter, domain.ReportMessage, r.message)
	if err != nil {
		t.Fatal(err)
	}
	handler := app.NewResolveReportHandler(db.New(r.pool, r.gen, r.clock), r.gen, r.clock)
	wantCode(t, handler.Handle(asAdmin(t), app.ResolveReport{ReportID: id, Admin: admin}), errs.CodeReasonRequired)
	if got := r.status(t, id); got != "open" {
		t.Fatalf("status after a refused resolve = %q, want open", got)
	}
}
