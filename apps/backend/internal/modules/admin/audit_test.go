package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type auditFixture struct {
	now  time.Time
	pool *pgxpool.Pool
	ids  *testkit.IDs
	uow  *db.UnitOfWork
}

func newAuditFixture(t *testing.T) auditFixture {
	t.Helper()
	g := testkit.NewIDs(1)
	pool := testkit.DB(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	return auditFixture{now: now, pool: pool, ids: g, uow: db.New(pool, g, testkit.NewClock(now))}
}

func (f auditFixture) admin() ids.UserID { return ids.UserIDFrom(f.ids.NewV7()) }

func (f auditFixture) action(
	t *testing.T, by ids.UserID, kind events.AdminActionKind, target events.AdminTargetType, targetID string,
) events.AdminAction {
	t.Helper()
	reason, err := events.NewReason("spam")
	if err != nil {
		t.Fatal(err)
	}
	e, err := events.NewAdminAction(f.ids.NewV7(), by, kind, target, targetID, reason,
		map[string]any{"flagged_at": nil}, map[string]any{"flagged_at": f.now})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (f auditFixture) audit(ctx context.Context, e events.AdminAction, at time.Time) error {
	return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return adapters.Audit{}.Handle(ctx, tx, e, at)
	})
}

type auditRow struct {
	ID         string
	AdminID    string
	Action     string
	TargetType string
	TargetID   string
	Reason     string
	Before     string
	After      string
	ApprovedBy string
	CreatedAt  time.Time
}

func (f auditFixture) rows(t *testing.T) []auditRow {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT id::text, admin_id::text, action, target_type, target_id, reason,
		before::text, after::text, coalesce(approved_by::text, ''), created_at FROM admin_actions ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[auditRow])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAudit_writesTheRowFromTheEventAndKeepsTheFirstDeliveryUnderRedelivery(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	by := f.admin()
	e := f.action(t, by, events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-1")
	approver := f.ids.NewV7()
	e.ApprovedBy = &approver
	first := f.now.Add(time.Second)
	for _, at := range []time.Time{first, f.now.Add(time.Minute)} {
		if err := f.audit(t.Context(), e, at); err != nil {
			t.Fatal(err)
		}
	}
	got := f.rows(t)
	if len(got) != 1 {
		t.Fatalf("admin_actions rows = %+v, want one for two deliveries of one event", got)
	}
	row := got[0]
	if !row.CreatedAt.Equal(first) {
		t.Fatalf("created_at = %s, want the first delivery %s", row.CreatedAt, first)
	}
	row.CreatedAt = time.Time{}
	want := auditRow{
		ID: e.ActionID.String(), AdminID: by.String(), Action: "ping_flag", TargetType: "system_ping",
		TargetID: "ping-1", Reason: "spam", Before: `{"flagged_at": null}`,
		After: `{"flagged_at": "2026-03-01T12:00:00Z"}`, ApprovedBy: approver.String(),
	}
	if row != want {
		t.Fatalf("row = %+v, want %+v", row, want)
	}
}

func TestAudit_storesNoApproverWhenTheEventHasNone(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	e := f.action(t, f.admin(), events.AdminActionGlobalPause, events.AdminTargetGlobal, "global")
	if err := f.audit(t.Context(), e, f.now); err != nil {
		t.Fatal(err)
	}
	if got := f.rows(t); len(got) != 1 || got[0].ApprovedBy != "" {
		t.Fatalf("rows = %+v, want one row with a null approved_by", got)
	}
}

func TestAudit_returnsTheWriteError(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE admin_actions RENAME TO admin_actions_gone`); err != nil {
		t.Fatal(err)
	}
	e := f.action(t, f.admin(), events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-1")
	if err := f.audit(t.Context(), e, f.now); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("audit err = %v, want internal", err)
	}
}

func TestModule_registersTheAuditConsumerOnTheAdminDurable(t *testing.T) {
	t.Parallel()
	consumers := admin.New(module.Deps{}).Consumers()
	if len(consumers) != 1 || consumers[0].Durable != "admin" || len(consumers[0].Handlers) != 1 {
		t.Fatalf("consumers = %+v, want the admin durable with one handler", consumers)
	}
	if h := consumers[0].Handlers[0]; h.Name != "admin.audit" || h.Type() != events.TypeAdminAction {
		t.Fatalf("handler = %s on %s, want admin.audit on admin.action", h.Name, h.Type())
	}
}
