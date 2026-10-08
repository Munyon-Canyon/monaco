package admin_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_ActionsListsTheAuditRowsOfOneTarget(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	by := f.admin()
	flagged := f.action(t, by, events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-1")
	other := f.action(t, by, events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-2")
	for _, e := range []events.AdminAction{flagged, other} {
		if err := f.audit(t.Context(), e, f.now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := admin.New(module.Deps{Pool: f.pool}).Actions().RecentActions(t.Context(), "system_ping", "ping-1", 5)
	if err != nil || len(got) != 1 {
		t.Fatalf("RecentActions = %+v, %v, want the one row of ping-1", got, err)
	}
	if a := got[0]; a.ID != flagged.ActionID || a.AdminID != by.UUID() || a.Kind != "ping_flag" ||
		a.Reason != "spam" || !a.At.Equal(f.now) {
		t.Fatalf("action = %+v, want ping_flag by the admin for spam at %s", a, f.now)
	}
}

func TestModule_ActionsReportsDBUnavailableWhenTheDatabaseIsDown(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := admin.New(module.Deps{Pool: f.pool}).
		Actions().
		RecentActions(ctx, "proposal", "id", 1); errs.CodeOf(
		err,
	) !=
		errs.CodeDBUnavailable {
		t.Fatalf("RecentActions = %v, want db_unavailable", err)
	}
}
