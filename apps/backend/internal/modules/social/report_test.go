package social_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type reportFixture struct {
	commentFixture
	users    *fakes.Identity
	cabals   app.Cabals
	report   *app.CreateReportHandler
	reporter ids.UserID
	target   ids.UserID
	deleted  ids.UserID
	message  uuid.UUID
	comment  uuid.UUID
}

type failingCabals struct{}

func (failingCabals) Cabal(context.Context, ids.CabalID) (cabalport.CabalView, error) {
	return cabalport.CabalView{}, errs.New(errs.CodeUpstreamUnavailable, "test")
}

func newReportFixture(t *testing.T) reportFixture {
	t.Helper()
	c := newCommentFixture(t)
	f := reportFixture{
		commentFixture: c,
		reporter:       c.cabal.Members[0].ID,
		target:         c.cabal.Members[1].ID,
		deleted:        ids.NewUserID(c.gen),
		cabals:         cabal.New(module.Deps{Pool: c.pool}).Queries(),
	}
	f.users = fakes.NewIdentity([]identity.UserCard{
		{ID: f.reporter, Handle: "reporter", AccountStatus: identity.AccountActive},
		{ID: f.target, Handle: "target", AccountStatus: identity.AccountBanned},
		{ID: f.deleted, AccountStatus: identity.AccountDeleted, Deleted: true},
	}, nil)
	f.report = f.handler(f.cabals)
	f.message = c.gen.NewV7()
	if _, err := c.pool.Exec(t.Context(),
		`INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at) VALUES ($1, $2, $3, 'hi', now())`,
		f.message, c.cabal.ID.UUID(), f.target.UUID()); err != nil {
		t.Fatal(err)
	}
	f.comment = f.mustComment(t, f.item(t, feed.KindTrade), f.target, "buy", uuid.Nil).ID
	return f
}

func (f reportFixture) handler(cabals app.Cabals) *app.CreateReportHandler {
	return app.NewCreateReportHandler(app.CreateReportDeps{
		UoW: db.New(f.pool, f.gen, f.clock), Reads: f.pool, Users: f.users, Cabals: cabals, IDs: f.gen, Clock: f.clock,
	})
}

func (f reportFixture) file(
	t *testing.T,
	reporter ids.UserID,
	kind domain.ReportKind,
	target uuid.UUID,
) (uuid.UUID, error) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+reporter.String())
	return f.report.Handle(ctx, app.CreateReport{
		Reporter: reporter, Kind: kind, TargetID: target, Reason: domain.ReasonAbuse, Note: "rude",
	})
}

func (f reportFixture) targets() map[domain.ReportKind]uuid.UUID {
	return map[domain.ReportKind]uuid.UUID{
		domain.ReportMessage: f.message, domain.ReportComment: f.comment,
		domain.ReportUser: f.target.UUID(), domain.ReportCabal: f.cabal.ID.UUID(),
	}
}

func (f reportFixture) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "user:"+f.reporter.String())
}

func (f reportFixture) reports(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM reports`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f reportFixture) reportEvents(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM events WHERE type = 'report.created'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateReport_storesTheReportAndAppendsReportCreatedWithoutTheNote(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	for kind, target := range f.targets() {
		id, err := f.file(t, f.reporter, kind, target)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		var (
			reason, note, status string
			payload              []byte
			aggregate            uuid.UUID
			actor                string
		)
		err = f.pool.QueryRow(t.Context(),
			`SELECT r.reason, r.note, r.status, e.payload, e.aggregate_id, e.actor_type || ':' || e.actor_id
			 FROM reports r, events e WHERE r.id = $1 AND e.aggregate_id = r.id AND e.type = 'report.created'`, id).
			Scan(&reason, &note, &status, &payload, &aggregate, &actor)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		var got events.ReportCreated
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatal(err)
		}
		want := events.ReportCreated{
			V: 1, ReportID: id, ReporterID: f.reporter.UUID(), Kind: string(kind), TargetID: target, Reason: "abuse",
		}
		if got != want || reason != "abuse" || note != "rude" || status != "open" || aggregate != id ||
			actor != "user:"+f.reporter.String() || strings.Contains(string(payload), "rude") {
			t.Fatalf(
				"%s: event %s = %+v, row %s/%s/%s, actor %s; want %+v",
				kind,
				payload,
				got,
				reason,
				note,
				status,
				actor,
				want,
			)
		}
	}
}

func TestCreateReport_aRepeatOnTheSameOpenTargetReturnsTheSameIdAndAppendsNothing(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	first, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	if err != nil || again != first {
		t.Fatalf("repeat = %s, %v; want %s", again, err, first)
	}
	other, err := f.file(t, f.target, domain.ReportUser, f.reporter.UUID())
	if err != nil || other == first {
		t.Fatalf("another reporter = %s, %v; want its own report", other, err)
	}
	if n := f.reportEvents(t); f.reports(t) != 2 || n != 2 {
		t.Fatalf("reports = %d, events = %d, want 2 and 2", f.reports(t), n)
	}
}

func TestCreateReport_aDeletedMessageStillCounts(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabal_messages SET deleted_at = now()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.file(t, f.reporter, domain.ReportMessage, f.message); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReport_aMissingTargetIsReportTargetNotFound(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	missing := map[domain.ReportKind]uuid.UUID{
		domain.ReportMessage: f.gen.NewV7(), domain.ReportComment: f.gen.NewV7(), domain.ReportCabal: f.gen.NewV7(),
	}
	for kind, target := range missing {
		_, err := f.file(t, f.reporter, kind, target)
		wantCode(t, err, errs.CodeReportTargetNotFound)
	}
	for _, user := range []uuid.UUID{f.deleted.UUID(), f.gen.NewV7()} {
		_, err := f.file(t, f.reporter, domain.ReportUser, user)
		wantCode(t, err, errs.CodeReportTargetNotFound)
	}
	if n := f.reportEvents(t); f.reports(t) != 0 || n != 0 {
		t.Fatalf("reports = %d, events = %d, want none", f.reports(t), n)
	}
}

func TestCreateReport_refusesInvalidInput(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	tests := []app.CreateReport{
		{Kind: "proposal", Reason: domain.ReasonSpam},
		{Kind: domain.ReportUser, Reason: "harassment"},
		{Kind: domain.ReportUser, Reason: domain.ReasonSpam, Note: strings.Repeat("é", domain.MaxReportNote+1)},
	}
	for _, cmd := range tests {
		cmd.Reporter, cmd.TargetID = f.reporter, f.target.UUID()
		_, err := f.report.Handle(f.ctx(t), cmd)
		wantCode(t, err, errs.CodeInvalidInput)
	}
	ok := app.CreateReport{
		Reporter: f.reporter, Kind: domain.ReportUser, TargetID: f.target.UUID(), Reason: domain.ReasonOther,
		Note: strings.Repeat("é", domain.MaxReportNote),
	}
	if _, err := f.report.Handle(f.ctx(t), ok); err != nil {
		t.Fatalf("a note of exactly 500 characters: %v", err)
	}
}

func TestCreateReport_storesNoNoteWhenNoneIsGiven(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	cmd := app.CreateReport{
		Reporter: f.reporter, Kind: domain.ReportUser, TargetID: f.target.UUID(), Reason: domain.ReasonSpam,
	}
	if _, err := f.report.Handle(f.ctx(t), cmd); err != nil {
		t.Fatal(err)
	}
	var isNull bool
	if err := f.pool.QueryRow(t.Context(), `SELECT note IS NULL FROM reports`).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("note IS NULL = %v, %v; want true", isNull, err)
	}
}

func TestCreateReport_returnsTheLookupFailures(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	_, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	f.report = f.handler(failingCabals{})
	_, err = f.file(t, f.reporter, domain.ReportCabal, f.cabal.ID.UUID())
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestCreateReport_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	for kind, table := range map[domain.ReportKind]string{
		domain.ReportMessage: "cabal_messages", domain.ReportComment: "feed_comments",
	} {
		if _, err := f.pool.Exec(t.Context(), `DROP TABLE `+table+` CASCADE`); err != nil {
			t.Fatal(err)
		}
		_, err := f.file(t, f.reporter, kind, f.targets()[kind])
		wantCode(t, err, errs.CodeInternal)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE reports CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	wantCode(t, err, errs.CodeInternal)
}

func TestCreateReport_failsAndRollsBackWhenTheEventCannotBeAppended(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE events CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID()); err == nil {
		t.Fatal("CreateReport = nil, want the append failure")
	}
	if n := f.reports(t); n != 0 {
		t.Fatalf("reports = %d, want the insert rolled back", n)
	}
}

func TestCreateReport_failsWhenTheOpenReportCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	for _, stmt := range []string{
		`CREATE FUNCTION skip_report() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`,
		`CREATE TRIGGER skip_report BEFORE INSERT ON reports FOR EACH ROW EXECUTE FUNCTION skip_report()`,
	} {
		if _, err := f.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID())
	wantCode(t, err, errs.CodeInternal)
}
