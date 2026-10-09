package admin_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestApprovals_List_FiltersByStatusAndPages(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	approvals := make([]any, 0, 3)
	for range 3 {
		approvals = append(approvals, f.request(t, testkit.NewCabal(t, f.pool).ID, "operator", http.StatusOK)["id"])
	}
	f.decide(t, approvals[0], "reject", "operator2", http.StatusOK)
	list := func(query string) map[string]any {
		return decodeBody(t, adminRequest(t, f.h, "/v1/admin/approvals"+query, "viewer"), http.StatusOK)
	}
	if got := list("")["items"].([]any); len(got) != 2 {
		t.Fatalf("pending = %v, want two", got)
	}
	if got := list("?status=rejected")["items"].([]any); len(got) != 1 {
		t.Fatalf("rejected = %v, want one", got)
	}
	page := list("?limit=1")
	items := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != approvals[2] || page["next_cursor"] != approvals[2] {
		t.Fatalf("first page = %v, want the newest request and its cursor", page)
	}
	next := list("?limit=1&cursor=" + page["next_cursor"].(string))
	if got := next["items"].([]any); len(got) != 1 || got[0].(map[string]any)["id"] != approvals[1] {
		t.Fatalf("second page = %v", next)
	}
	if last := list("?limit=5"); last["next_cursor"] != nil {
		t.Fatalf("last page cursor = %v, want null", last["next_cursor"])
	}
}

func TestApprovals_List_StoreFailure(t *testing.T) {
	t.Parallel()
	f := newApprovalFixture(t)
	f.exec(t, `DROP TABLE admin_approvals`)
	_, err := adapters.HTTP{Pool: f.pool}.GetAdminApprovals(t.Context(), adminapi.GetAdminApprovalsRequestObject{})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("list with the table gone = %v, want db_unavailable", err)
	}
}
