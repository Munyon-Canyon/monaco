package social_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f reportFixture) routes() adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(f.users)))
}

func (f reportFixture) asAdmin(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorAdmin, ID: f.reporter.String(), Role: "moderator"})
}

func reportBody(kind api.ReportRequestKind, target uuid.UUID, note *string) *api.ReportRequest {
	return &api.ReportRequest{Kind: kind, TargetId: target, Reason: api.ReportRequestReason("spam"), Note: note}
}

func TestPostReport_answers201WithTheSameIdForARepeat(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	note := "same link to everyone"
	got := make([]uuid.UUID, 0, 2)
	for _, body := range []*api.ReportRequest{
		reportBody("user", f.target.UUID(), &note), reportBody("user", f.target.UUID(), nil),
	} {
		res, err := f.routes().PostReport(asUser(t.Context(), f.reporter), api.PostReportRequestObject{Body: body})
		if err != nil {
			t.Fatal(err)
		}
		created, ok := res.(api.PostReport201JSONResponse)
		if !ok {
			t.Fatalf("response = %#v, want 201", res)
		}
		got = append(got, created.Id)
	}
	if got[0] != got[1] || f.reports(t) != 1 || f.reportEvents(t) != 1 {
		t.Fatalf(
			"ids = %v, reports = %d, events = %d; want one report and one event",
			got,
			f.reports(t),
			f.reportEvents(t),
		)
	}
}

func TestPostReport_refusesWhatItCannotFile(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	ctx := asUser(t.Context(), f.reporter)
	_, err := f.routes().PostReport(ctx, api.PostReportRequestObject{})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes().PostReport(ctx, api.PostReportRequestObject{Body: reportBody("proposal", f.target.UUID(), nil)})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes().PostReport(ctx, api.PostReportRequestObject{Body: reportBody("user", f.gen.NewV7(), nil)})
	wantCode(t, err, errs.CodeReportTargetNotFound)
	_, err = f.routes().
		PostReport(t.Context(), api.PostReportRequestObject{Body: reportBody("user", f.target.UUID(), nil)})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestGetAdminReports_refusesACallerThatIsNotAnAdmin(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	for _, ctx := range []context.Context{t.Context(), asUser(t.Context(), f.reporter)} {
		_, err := f.routes().GetAdminReports(ctx, api.GetAdminReportsRequestObject{})
		wantCode(t, err, errs.CodeAdminForbidden)
	}
}

func (f reportFixture) fileFour(t *testing.T) []uuid.UUID {
	t.Helper()
	filed := make([]uuid.UUID, 0, 4)
	for _, kind := range []domain.ReportKind{
		domain.ReportMessage, domain.ReportComment, domain.ReportUser, domain.ReportCabal,
	} {
		f.clock.Advance(time.Second)
		id, err := f.file(t, f.reporter, kind, f.targets()[kind])
		if err != nil {
			t.Fatal(err)
		}
		filed = append(filed, id)
	}
	return filed
}

func wantListed(t *testing.T, item api.AdminReport, id uuid.UUID) {
	t.Helper()
	ok := item.Id == id && item.Reporter.Handle == "reporter" && item.Status == "open" && item.Reason == "abuse"
	if !ok || item.Note == nil || *item.Note != "rude" || item.TargetId == uuid.Nil {
		t.Fatalf("item = %+v, want open report %s by reporter", item, id)
	}
}

func TestGetAdminReports_listsOpenReportsOldestFirstAndPages(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	filed := f.fileFour(t)
	admin := f.asAdmin(t.Context())
	limit := 3
	res, err := f.routes().GetAdminReports(admin, api.GetAdminReportsRequestObject{
		Params: api.GetAdminReportsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := res.(api.GetAdminReports200JSONResponse)
	if len(first.Items) != 3 || first.NextCursor == nil {
		t.Fatalf("first page = %d items, cursor %v; want 3 and a cursor", len(first.Items), first.NextCursor)
	}
	for i, item := range first.Items {
		wantListed(t, item, filed[i])
	}
	res, err = f.routes().GetAdminReports(admin, api.GetAdminReportsRequestObject{
		Params: api.GetAdminReportsParams{Limit: &limit, Cursor: first.NextCursor},
	})
	if err != nil {
		t.Fatal(err)
	}
	second := res.(api.GetAdminReports200JSONResponse)
	if len(second.Items) != 1 || second.Items[0].Id != filed[3] || second.NextCursor != nil {
		t.Fatalf("second page = %+v, want the last report and no cursor", second)
	}
}

func TestGetAdminReports_filtersByStatusAndBlanksADeletedReporter(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	f.fileFour(t)
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO reports (id, reporter_id, kind, target_id, reason, status, created_at)
		 VALUES ($1, $2, 'user', $3, 'other', 'resolved', now())`,
		f.gen.NewV7(), f.deleted.UUID(), f.target.UUID()); err != nil {
		t.Fatal(err)
	}
	resolved := api.GetAdminReportsParamsStatus("resolved")
	res, err := f.routes().GetAdminReports(f.asAdmin(t.Context()), api.GetAdminReportsRequestObject{
		Params: api.GetAdminReportsParams{Status: &resolved},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := res.(api.GetAdminReports200JSONResponse)
	if len(got.Items) != 1 || got.Items[0].Status != "resolved" || got.Items[0].Reporter.Handle != "" ||
		got.Items[0].Reporter.UserId != f.deleted.UUID() || got.Items[0].Note != nil {
		t.Fatalf("resolved reports = %+v, want the deleted reporter's one with no handle or note", got.Items)
	}
}

func TestGetAdminReports_answersAnEmptyPageWithoutLookingUpUsers(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	f.users.Fail("UsersByID", errs.New(errs.CodeInternal, "test"))
	res, err := f.routes().GetAdminReports(f.asAdmin(t.Context()), api.GetAdminReportsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(api.GetAdminReports200JSONResponse); got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("items = %v, want an empty list", got.Items)
	}
}

func TestGetAdminReports_refusesABadQueryAndPassesFailuresThrough(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	f.fileFour(t)
	admin := f.asAdmin(t.Context())
	zero, bogus, junk := 0, api.GetAdminReportsParamsStatus("bogus"), "not-a-cursor"
	for _, params := range []api.GetAdminReportsParams{{Limit: &zero}, {Status: &bogus}, {Cursor: &junk}} {
		_, err := f.routes().GetAdminReports(admin, api.GetAdminReportsRequestObject{Params: params})
		wantCode(t, err, errs.CodeInvalidInput)
	}
	f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	_, err := f.routes().GetAdminReports(admin, api.GetAdminReportsRequestObject{})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE reports CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err = f.routes().GetAdminReports(admin, api.GetAdminReportsRequestObject{})
	wantCode(t, err, errs.CodeInternal)
}
