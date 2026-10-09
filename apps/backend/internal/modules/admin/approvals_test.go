package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	operatorA = "019cc330-1111-7000-8000-000000000001"
	operatorB = "019cc330-1111-7000-8000-000000000005"
)

type approvalFixture struct {
	pool  *pgxpool.Pool
	clock *testkit.Clock
	ids   *testkit.IDs
	uow   *db.UnitOfWork
	cabal app.CabalStatuses
	h     http.Handler
}

func newApprovalFixture(t *testing.T) approvalFixture {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	gen := testkit.NewIDs(71)
	uow := db.New(pool, gen, clk)
	deps := module.Deps{Pool: pool, Clock: clk, UoW: uow, IDs: gen}
	return approvalFixture{
		pool: pool, clock: clk, ids: gen, uow: uow, cabal: cabal.New(deps).Queries(), h: wiredAdminHandler(t, deps),
	}
}

func (f approvalFixture) post(t *testing.T, path, token, body string, want int) map[string]any {
	t.Helper()
	w := adminPost(t, f.h, path, token, body)
	return decodeBody(t, w, want)
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d %s, want %d", w.Code, w.Body, want)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func (f approvalFixture) request(t *testing.T, cabal ids.CabalID, token string, want int) map[string]any {
	t.Helper()
	return f.post(t, "/v1/admin/cabals/"+cabal.String()+"/ban-requests", token, `{"reason":"scam cabal"}`, want)
}

func (f approvalFixture) decide(t *testing.T, id any, verb, token string, want int) map[string]any {
	t.Helper()
	return f.post(t, "/v1/admin/approvals/"+id.(string)+"/"+verb, token, `{"reason":"confirmed"}`, want)
}

func (f approvalFixture) events(t *testing.T) (n int) {
	t.Helper()
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type LIKE 'admin.%'`).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantCode(t *testing.T, body map[string]any, code errs.Code) {
	t.Helper()
	if body["code"] != string(code) {
		t.Fatalf("body = %v, want code %s", body, code)
	}
}

func TestApprovals_Request_OpensAPendingRequestForTwentyFourHours(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	cabal := testkit.NewCabal(t, f.pool)
	body := f.request(t, cabal.ID, "operator", http.StatusOK)
	if body["status"] != "pending" || body["action"] != "cabal_ban" || body["target_id"] != cabal.ID.String() ||
		body["requested_by"] != operatorA || body["reason"] != "scam cabal" || body["decided_by"] != nil ||
		body["expires_at"] != "2026-03-02T12:00:00Z" || body["created_at"] != "2026-03-01T12:00:00Z" {
		t.Fatalf("body = %v", body)
	}
	if got := f.events(t); got != 1 {
		t.Fatalf("admin events = %d, want the request", got)
	}
}

func TestApprovals_Request_Refusals(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	cabal := testkit.NewCabal(t, f.pool)
	f.request(t, cabal.ID, "viewer", http.StatusForbidden)
	wantCode(t, f.request(t, ids.CabalIDFrom(f.ids.NewV7()), "operator", http.StatusNotFound), errs.CodeCabalNotFound)
	f.request(t, cabal.ID, "operator", http.StatusOK)
	wantCode(t, f.request(t, cabal.ID, "operator2", http.StatusConflict), errs.CodeApprovalAlreadyPending)
	other := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, other.ID.UUID())
	wantCode(t, f.request(t, other.ID, "operator", http.StatusConflict), errs.CodeCabalNotActive)
	short := f.post(t, "/v1/admin/cabals/"+cabal.ID.String()+"/ban-requests", "operator", `{"reason":"x"}`, 400)
	wantCode(t, short, errs.CodeReasonRequired)
	if got := f.events(t); got != 1 {
		t.Fatalf("admin events = %d, want only the first request", got)
	}
}

func TestApprovals_Approve_NeedsADifferentOperator(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	cabal := testkit.NewCabal(t, f.pool)
	id := f.request(t, cabal.ID, "operator", http.StatusOK)["id"]
	wantCode(t, f.decide(t, id, "approve", "operator", http.StatusForbidden), errs.CodeSameApprover)
	short := f.post(
		t,
		"/v1/admin/approvals/"+id.(string)+"/approve",
		"operator2",
		`{"reason":"x"}`,
		http.StatusBadRequest,
	)
	wantCode(t, short, errs.CodeReasonRequired)
	if got := f.events(t); got != 1 {
		t.Fatalf("admin events = %d, want no approval event", got)
	}
	body := f.decide(t, id, "approve", "operator2", http.StatusOK)
	if body["status"] != "approved" || body["decided_by"] != operatorB || body["decided_reason"] != "confirmed" ||
		body["decided_at"] != "2026-03-01T12:00:00Z" {
		t.Fatalf("body = %v", body)
	}
	event := f.approvedEvent(t)
	if event["approval_id"] != id || event["cabal_id"] != cabal.ID.String() || event["requested_by"] != operatorA ||
		event["approved_by"] != operatorB || event["reason"] != "scam cabal" {
		t.Fatalf("event = %v", event)
	}
	wantCode(t, f.decide(t, id, "approve", "operator2", http.StatusConflict), errs.CodeApprovalNotPending)
	wantCode(t, f.decide(t, id, "reject", "operator2", http.StatusConflict), errs.CodeApprovalNotPending)
}

func TestApprovals_Reject_AnyOperatorMayReject(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	cabal := testkit.NewCabal(t, f.pool)
	id := f.request(t, cabal.ID, "operator", http.StatusOK)["id"]
	body := f.decide(t, id, "reject", "operator", http.StatusOK)
	if body["status"] != "rejected" || body["decided_by"] != operatorA {
		t.Fatalf("body = %v", body)
	}
	if got := f.events(t); got != 1 {
		t.Fatalf("admin events = %d, want no approval event", got)
	}
	wantCode(t, f.decide(t, id, "approve", "operator2", http.StatusConflict), errs.CodeApprovalNotPending)
	f.request(t, cabal.ID, "operator", http.StatusOK)
	f.decide(t, uuid.NewString(), "reject", "operator", http.StatusNotFound)
	f.decide(t, uuid.NewString(), "approve", "operator2", http.StatusNotFound)
}

func TestApprovals_Expiry_ARequestPastTwentyFourHoursCannotBeDecided(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	cabal := testkit.NewCabal(t, f.pool)
	id := f.request(t, cabal.ID, "operator", http.StatusOK)["id"]
	f.clock.Advance(app.ApprovalTTL)
	wantCode(t, f.decide(t, id, "approve", "operator2", http.StatusConflict), errs.CodeApprovalExpired)
	wantCode(t, f.decide(t, id, "reject", "operator2", http.StatusConflict), errs.CodeApprovalExpired)
	report, err := app.NewApprovalsExpirePoller(f.uow, f.clock).Tick(systemCtx(t))
	if err != nil || report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want one expired", report, err)
	}
	wantCode(t, f.decide(t, id, "approve", "operator2", http.StatusConflict), errs.CodeApprovalExpired)
	if n := f.count(t, "admin_approvals WHERE status = 'expired'"); n != 1 {
		t.Fatalf("expired requests = %d, want 1", n)
	}
	f.request(t, cabal.ID, "operator", http.StatusOK)
}

func TestApprovals_ExpirePoller_LeavesLiveRequestsAlone(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	f.request(t, testkit.NewCabal(t, f.pool).ID, "operator", http.StatusOK)
	f.clock.Advance(time.Hour)
	f.request(t, testkit.NewCabal(t, f.pool).ID, "operator", http.StatusOK)
	f.clock.Advance(app.ApprovalTTL - time.Hour)
	p := app.NewApprovalsExpirePoller(f.uow, f.clock)
	first, err := p.Tick(systemCtx(t))
	again, _ := p.Tick(systemCtx(t))
	if err != nil || first.Changed != 1 || again.Changed != 0 || p.Name() != "admin.approvals_expire" ||
		p.Interval() != 5*time.Minute {
		t.Fatalf("Tick = %+v, %v then %+v on %s every %s, want one expired", first, err, again, p.Name(), p.Interval())
	}
	if _, err := p.Tick(cancelled(t)); err == nil {
		t.Fatal("Tick on a cancelled context = nil error")
	}
}

func systemCtx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:poller.admin.approvals_expire")
}

func cancelled(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

type statusError struct{ err error }

func (s statusError) Status(context.Context, ids.CabalID) (cabalport.Status, error) { return "", s.err }

type commandEnv struct {
	f      approvalFixture
	cabal  testkit.SeededCabal
	admin  ids.UserID
	reason events.Reason
}

func newCommandEnv(t *testing.T) commandEnv {
	t.Helper()
	f := newApprovalFixture(t)
	admin := ids.UserIDFrom(f.ids.NewV7())
	reason, _ := events.NewReason("scam cabal")
	return commandEnv{
		f: f, cabal: testkit.NewCabal(t, f.pool), admin: admin, reason: reason,
	}
}

func (e commandEnv) ctx(t *testing.T) context.Context {
	t.Helper()
	actor := auth.Actor{Kind: auth.ActorAdmin, ID: e.admin.String(), Role: "operator"}
	return auth.WithActor(systemCtx(t), actor)
}

func (e commandEnv) request(ctx context.Context, cabals app.CabalStatuses) (app.Approval, error) {
	a := app.NewApprovals(e.f.uow, cabals, e.f.ids, e.f.clock)
	return a.RequestCabalBan(ctx, app.RequestCabalBan{CabalID: e.cabal.ID, AdminID: e.admin, Reason: e.reason})
}

func (e commandEnv) approve(ctx context.Context, id uuid.UUID) error {
	a := app.NewApprovals(e.f.uow, e.f.cabal, e.f.ids, e.f.clock)
	_, err := a.Approve(ctx, app.DecideApproval{ID: id, AdminID: ids.UserIDFrom(e.f.ids.NewV7()), Reason: e.reason})
	return err
}

func TestApprovals_Commands_RollBackWithoutAnActorAndSurfaceStoreFailures(t *testing.T) {
	t.Parallel()
	e := newCommandEnv(t)
	if _, err := e.request(t.Context(), e.f.cabal); err == nil || e.f.count(t, "admin_approvals") != 0 {
		t.Fatalf("request without an actor = %v, want the append to fail and the insert to roll back", err)
	}
	broken := statusError{err: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := e.request(e.ctx(t), broken); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("status failure = %v, want db_unavailable", err)
	}
	row, err := e.request(e.ctx(t), e.f.cabal)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.approve(t.Context(), row.ID); err == nil || e.f.approvalStatus(t, row.ID) != app.ApprovalPending {
		t.Fatalf("approve without an actor = %v, want the append to fail and the approval to roll back", err)
	}
	if err := e.approve(cancelled(t), row.ID); err == nil {
		t.Fatal("approve on a cancelled context = nil error")
	}
	e.f.exec(t, `DROP TABLE admin_approvals`)
	if err := e.approve(e.ctx(t), row.ID); err == nil || errs.CodeOf(err) == errs.CodeNotFound {
		t.Fatalf("approve with the table gone = %v, want the store failure", err)
	}
}

func TestApprovals_Commands_StoreRefusesAWrite(t *testing.T) {
	t.Parallel()
	for _, when := range []string{"INSERT", "UPDATE"} {
		t.Run(when, func(t *testing.T) {
			t.Parallel()
			e := newCommandEnv(t)
			var row app.Approval
			if when == "UPDATE" {
				var err error
				if row, err = e.request(e.ctx(t), e.f.cabal); err != nil {
					t.Fatal(err)
				}
			}
			e.f.exec(
				t,
				`CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`,
			)
			e.f.exec(
				t,
				`CREATE TRIGGER refuse BEFORE `+when+` ON admin_approvals FOR EACH ROW EXECUTE FUNCTION refuse()`,
			)
			err := e.approve(e.ctx(t), row.ID)
			if when == "INSERT" {
				_, err = e.request(e.ctx(t), e.f.cabal)
			}
			if err == nil {
				t.Fatal("error = nil, want the store failure")
			}
		})
	}
}

func TestApprovals_Request_FailsWhenItCannotSweepAPastDueRequest(t *testing.T) {
	t.Parallel()
	e := newCommandEnv(t)
	if _, err := e.request(e.ctx(t), e.f.cabal); err != nil {
		t.Fatal(err)
	}
	e.f.clock.Advance(app.ApprovalTTL)
	e.f.exec(t, `CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`)
	e.f.exec(t, `CREATE TRIGGER refuse BEFORE UPDATE ON admin_approvals FOR EACH ROW EXECUTE FUNCTION refuse()`)
	if _, err := e.request(e.ctx(t), e.f.cabal); err == nil {
		t.Fatal("request that cannot sweep the past-due one = nil error")
	}
}

func (f approvalFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f approvalFixture) count(t *testing.T, table string) (n int) {
	t.Helper()
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f approvalFixture) approvalStatus(t *testing.T, id uuid.UUID) (status string) {
	t.Helper()
	q := `SELECT status FROM admin_approvals WHERE id = $1`
	if err := f.pool.QueryRow(t.Context(), q, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f approvalFixture) approvedEvent(t *testing.T) (event map[string]any) {
	t.Helper()
	var payload []byte
	q := `SELECT payload FROM events WHERE type = 'admin.cabal_ban_approved'`
	if err := f.pool.QueryRow(t.Context(), q).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event
}
