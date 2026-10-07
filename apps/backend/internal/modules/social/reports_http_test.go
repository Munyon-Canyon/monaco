package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f reportFixture) routes() adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(f.users)))
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
