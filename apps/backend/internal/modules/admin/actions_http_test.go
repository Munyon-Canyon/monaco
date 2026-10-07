package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
)

type actionItem struct {
	ID       string `json:"id"`
	TargetID string `json:"target_id"`
}

type actionPage struct {
	Items      []actionItem `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}

func (f auditFixture) list(t *testing.T, token, query string) (int, actionPage) {
	t.Helper()
	w := adminRequest(t, adminHandlerOn(t, f.pool), "/v1/admin/actions"+query, token)
	var page actionPage
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("response %s: %v", w.Body, err)
		}
	}
	return w.Code, page
}

func targets(page actionPage) []string {
	got := make([]string, len(page.Items))
	for i, item := range page.Items {
		got[i] = item.TargetID
	}
	return got
}

func TestAdminActions_aViewerReadsEveryColumnOfTheActionsOfOneTarget(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	by := f.admin()
	flag := f.action(t, by, events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-1")
	approver := f.ids.NewV7()
	flag.ApprovedBy = &approver
	other := f.action(t, by, events.AdminActionPingFlag, events.AdminTargetSystemPing, "ping-2")
	for _, e := range []events.AdminAction{flag, other} {
		if err := f.audit(t.Context(), e, f.now); err != nil {
			t.Fatal(err)
		}
	}
	const path = "/v1/admin/actions?target_type=system_ping&target_id=ping-1"
	w := adminRequest(t, adminHandlerOn(t, f.pool), path, "viewer")
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("response %s: %v", w.Body, err)
	}
	want := map[string]any{
		"id": flag.ActionID.String(), "admin_id": by.String(), "action": "ping_flag", "target_type": "system_ping",
		"target_id": "ping-1", "reason": "spam", "before": map[string]any{"flagged_at": nil},
		"after":       map[string]any{"flagged_at": "2026-03-01T12:00:00Z"},
		"approved_by": approver.String(), "created_at": "2026-03-01T12:00:00Z",
	}
	if w.Code != http.StatusOK || page.NextCursor != nil || len(page.Items) != 1 ||
		!reflect.DeepEqual(page.Items[0], want) {
		t.Fatalf("list = %d %+v cursor %v, want 200 with the one action %v", w.Code, page.Items, page.NextCursor, want)
	}
}

func TestAdminActions_aNullApproverAndAnEmptyPageAreNullAndEmpty(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	e := f.action(t, f.admin(), events.AdminActionGlobalPause, events.AdminTargetGlobal, "global")
	if err := f.audit(t.Context(), e, f.now); err != nil {
		t.Fatal(err)
	}
	h := adminHandlerOn(t, f.pool)
	if w := adminRequest(
		t,
		h,
		"/v1/admin/actions",
		"viewer",
	); !strings.Contains(
		w.Body.String(),
		`"approved_by":null`,
	) {
		t.Fatalf("body = %s, want an approved_by of null", w.Body)
	}
	w := adminRequest(t, h, "/v1/admin/actions?target_id=nobody", "viewer")
	if w.Code != http.StatusOK || w.Body.String() != `{"items":[],"next_cursor":null}`+"\n" {
		t.Fatalf("empty page = %d %q, want 200 with an empty items array and a null cursor", w.Code, w.Body)
	}
}

func TestAdminActions_aUserTokenIsForbidden(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	if code, _ := f.list(t, "user", "?target_type=system_ping"); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", code)
	}
}

func TestAdminActions_filtersNarrowAndCombineNewestFirst(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	a, b := f.admin(), f.admin()
	for _, e := range []events.AdminAction{
		f.action(t, a, events.AdminActionPingFlag, events.AdminTargetSystemPing, "p1"),
		f.action(t, a, events.AdminActionGlobalPause, events.AdminTargetGlobal, "g1"),
		f.action(t, b, events.AdminActionPingFlag, events.AdminTargetSystemPing, "p2"),
		f.action(t, b, events.AdminActionUserBan, events.AdminTargetUser, "u1"),
	} {
		if err := f.audit(t.Context(), e, f.now); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{"no filter", "", []string{"u1", "p2", "g1", "p1"}},
		{"admin", "?admin_id=" + a.String(), []string{"g1", "p1"}},
		{"action", "?action=ping_flag", []string{"p2", "p1"}},
		{"target type", "?target_type=user", []string{"u1"}},
		{"target id", "?target_id=p1", []string{"p1"}},
		{"action and admin", "?action=ping_flag&admin_id=" + b.String(), []string{"p2"}},
		{"target type and id", "?target_type=system_ping&target_id=u1", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, page := f.list(t, "viewer", tc.query)
			if code != http.StatusOK || !slices.Equal(targets(page), tc.want) {
				t.Fatalf("list %s = %d %v, want %v", tc.query, code, targets(page), tc.want)
			}
		})
	}
}

func TestAdminActions_aCursorPagesNewestFirstAndReturnsNoRowTwice(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	by := f.admin()
	want := []string{"t5", "t4", "t3", "t2", "t1"}
	for _, id := range slices.Backward(want) {
		if err := f.audit(
			t.Context(),
			f.action(t, by, events.AdminActionUserBan, events.AdminTargetUser, id),
			f.now,
		); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	query := "?limit=2"
	for pages := 0; ; pages++ {
		code, page := f.list(t, "viewer", query)
		if code != http.StatusOK || pages > len(want) {
			t.Fatalf("page %d = %d, want 200 and an end within %d pages", pages, code, len(want))
		}
		got = append(got, targets(page)...)
		if page.NextCursor == nil {
			break
		}
		if last := page.Items[len(page.Items)-1].ID; *page.NextCursor != last {
			t.Fatalf("next_cursor = %s, want the id of the last item %s", *page.NextCursor, last)
		}
		query = "?limit=2&cursor=" + *page.NextCursor
	}
	if !slices.Equal(got, want) {
		t.Fatalf("pages concatenated = %v, want %v", got, want)
	}
}

func TestAdminActions_aFullPageCarriesACursorAndTheNextPageIsEmpty(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	by := f.admin()
	for _, id := range []string{"t1", "t2"} {
		if err := f.audit(
			t.Context(),
			f.action(t, by, events.AdminActionUserBan, events.AdminTargetUser, id),
			f.now,
		); err != nil {
			t.Fatal(err)
		}
	}
	_, page := f.list(t, "viewer", "?limit=2")
	if len(page.Items) != 2 || page.NextCursor == nil {
		t.Fatalf("full page = %+v, want two items and a next cursor", page)
	}
	_, last := f.list(t, "viewer", "?limit=2&cursor="+*page.NextCursor)
	if len(last.Items) != 0 || last.NextCursor != nil {
		t.Fatalf("page after the full page = %+v, want it empty with a null cursor", last)
	}
}

func TestAdminActions_theDefaultPageHoldsFiftyActions(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO admin_actions
		(id, admin_id, action, target_type, target_id, reason, before, after, created_at)
		SELECT gen_random_uuid(), gen_random_uuid(), 'ping_flag', 'system_ping', n::text, 'spam', 'null', 'null', now()
		FROM generate_series(1, 51) n`); err != nil {
		t.Fatal(err)
	}
	_, page := f.list(t, "viewer", "")
	if len(page.Items) != 50 || page.NextCursor == nil {
		t.Fatalf("default page = %d items, cursor %v, want 50 items and a cursor", len(page.Items), page.NextCursor)
	}
}

func TestAdminActions_refusesAnOutOfRangeLimitAndAnUnknownFilterValue(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	for _, query := range []string{
		"?limit=0", "?limit=201", "?action=bogus", "?target_type=bogus", "?cursor=nope", "?admin_id=nope",
	} {
		if code, _ := f.list(t, "viewer", query); code != http.StatusBadRequest {
			t.Errorf("list %s = %d, want 400", query, code)
		}
	}
}

func TestAdminActionsAdapter_reportsAnUnavailableRead(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := adapters.HTTP{Pool: f.pool}.GetAdminActions(ctx, adminapi.GetAdminActionsRequestObject{})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("read error = %v, want db_unavailable", err)
	}
}
