package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f *letterFixture) handler(t *testing.T) http.Handler {
	t.Helper()
	return adminHandlerWith(t, module.Deps{
		Pool: f.pool, UoW: f.uow, Bus: f.bus.Conn, Clock: f.clock, IDs: f.ids,
	})
}

func adminPost(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func (f *letterFixture) seed(t *testing.T, seq int, consumer, status string) uuid.UUID {
	t.Helper()
	id := f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO dead_letters (id, stream_seq, consumer, handler, subject, code,
		error, letter, status, first_seen_at, last_seen_at) VALUES ($1, $2, $3, 'h', 's', 'c', 'e', '{"secret":1}', $4,
		$5, $5)`, id, seq, consumer, status, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	return id
}

type deadLetterPage struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

func (f *letterFixture) page(t *testing.T, h http.Handler, query string) deadLetterPage {
	t.Helper()
	w := adminRequest(t, h, "/v1/admin/dead-letters"+query, "viewer")
	var page deadLetterPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s (%v), want 200", query, w.Code, w.Body, err)
	}
	return page
}

func TestGetDeadLetters_aViewerReadsOpenLettersNewestFirstWithEveryColumnAndNeverTheStoredBody(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	f.tick(t)
	f.seed(t, 90, "notify", "resolved")
	h := f.handler(t)

	page := f.page(t, h, "")
	var id string
	if err := f.pool.QueryRow(t.Context(), `SELECT id::text FROM dead_letters WHERE stream_seq = 1`).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": id, "stream_seq": 1.0, "consumer": "analytics", "handler": "analytics.posthog.follow.created",
		"subject": f.bus.Conn.Subject("events.follow.created"), "event_id": event.String(),
		"code": "posthog_rejected", "error": "posthog.Capture: posthog_rejected", "occurrences": 1.0, "status": "open",
		"first_seen_at": "2026-03-01T12:00:00Z", "last_seen_at": "2026-03-01T12:00:00Z", "redriven_at": nil,
		"resolved_at": nil, "resolved_by": nil, "resolve_reason": nil,
	}
	if page.NextCursor != nil || len(page.Items) != 1 || !reflect.DeepEqual(page.Items[0], want) {
		t.Fatalf("page = %+v cursor %v, want the one open letter %v", page.Items, page.NextCursor, want)
	}
	if body := adminRequest(
		t,
		h,
		"/v1/admin/dead-letters",
		"viewer",
	).Body.String(); strings.Contains(
		body,
		"letter\"",
	) {
		t.Fatalf("body = %s, want no stored letter", body)
	}
}

func TestGetDeadLetters_filtersByStatusAndConsumerAndPagesByCursor(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	first := f.seed(t, 1, "analytics", "open")
	f.seed(t, 2, "notify", "open")
	third := f.seed(t, 3, "analytics", "open")
	f.seed(t, 4, "analytics", "redriven")
	f.seed(t, 5, "analytics", "resolved")
	h := f.handler(t)

	ids := func(page deadLetterPage) []string {
		got := make([]string, len(page.Items))
		for i, item := range page.Items {
			got[i], _ = item["consumer"].(string)
			got[i] += ":" + item["status"].(string)
		}
		return got
	}
	if got := ids(f.page(t, h, "?status=redriven")); !reflect.DeepEqual(got, []string{"analytics:redriven"}) {
		t.Fatalf("status=redriven = %v", got)
	}
	if got := ids(
		f.page(t, h, "?consumer=analytics"),
	); !reflect.DeepEqual(
		got,
		[]string{"analytics:open", "analytics:open"},
	) {
		t.Fatalf("consumer=analytics = %v, want its two open letters", got)
	}
	page := f.page(t, h, "?consumer=analytics&limit=1")
	if page.NextCursor == nil || *page.NextCursor != third.String() || len(page.Items) != 1 {
		t.Fatalf(
			"first page = %+v cursor %v, want the newest letter and its id as the cursor",
			page.Items,
			page.NextCursor,
		)
	}
	next := f.page(t, h, "?consumer=analytics&limit=1&cursor="+*page.NextCursor)
	if next.NextCursor == nil || *next.NextCursor != first.String() || next.Items[0]["id"] != first.String() {
		t.Fatalf("second page = %+v cursor %v, want the older letter", next.Items, next.NextCursor)
	}
	if last := f.page(
		t,
		h,
		"?consumer=analytics&cursor="+first.String(),
	); len(last.Items) != 0 ||
		last.NextCursor != nil {
		t.Fatalf("page after the oldest = %+v, want empty", last)
	}
}

func TestDeadLetterRoutes_anOperatorRedrivesAndDiscardsAndEveryoneElseIsRefused(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	f.tick(t)
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM dead_letters`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := f.handler(t)
	redrive, discard := "/v1/admin/dead-letters/"+id.String()+"/redrive", "/v1/admin/dead-letters/"+id.String()+"/discard"
	const body = `{"reason":"posthog fixed"}`

	for _, tc := range []struct {
		name, path, token, body string
		status                  int
		code                    string
	}{
		{"viewer redrive", redrive, "viewer", body, http.StatusForbidden, "admin_forbidden"},
		{"viewer discard", discard, "viewer", body, http.StatusForbidden, "admin_forbidden"},
		{"short reason", discard, "operator", `{"reason":"x"}`, http.StatusBadRequest, "reason_required"},
		{
			"unknown redrive", "/v1/admin/dead-letters/" + uuid.NewString() + "/redrive", "operator", body,
			http.StatusNotFound, "not_found",
		},
		{"redrive", redrive, "operator", body, http.StatusNoContent, ""},
		{"discard", discard, "operator", body, http.StatusNoContent, ""},
		{"discard again", discard, "operator", body, http.StatusConflict, "dead_letter_not_open"},
		{"redrive a discarded letter", redrive, "operator", body, http.StatusConflict, "dead_letter_not_open"},
	} {
		w := adminPost(t, h, tc.path, tc.token, tc.body)
		if w.Code != tc.status || (tc.code != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`)) {
			t.Fatalf("%s = %d %s, want %d %s", tc.name, w.Code, w.Body, tc.status, tc.code)
		}
	}
	if row := f.row(t, id); row.Status != "discarded" || row.RedrivenAt == nil {
		t.Fatalf("row = %+v, want discarded after the redrive", row)
	}
	if got := f.eventsStreamLength(t); got != 1 {
		t.Fatalf("EVENTS holds %d messages, want the one redrive", got)
	}
}

func TestGetDeadLetters_aRedrivenRowShowsWhoRedroveItWhenAndWhy(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	by := f.admin()
	if err := f.redriveHandler(f.bus.Conn).Handle(t.Context(),
		app.RedriveDeadLetter{ID: id, AdminID: by, Reason: f.reason(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE dead_letters SET resolved_at = $1`, f.clock.Now()); err != nil {
		t.Fatal(err)
	}

	page := f.page(t, f.handler(t), "?status=redriven")
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want the redriven row", page.Items)
	}
	item := page.Items[0]
	if item["resolved_by"] != by.String() || item["resolve_reason"] != "posthog fixed" ||
		item["redriven_at"] != "2026-03-01T12:00:00Z" || item["resolved_at"] != "2026-03-01T12:00:00Z" {
		t.Fatalf("item = %v, want the admin, reason and times the redrive recorded", item)
	}
}

func TestDeadLetterRoutes_failWhenTheTableIsGoneOrTheActorIsNotAnAdminUUID(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	h := f.handler(t)
	if w := adminPost(
		t,
		h,
		"/v1/admin/dead-letters/"+uuid.NewString()+"/redrive",
		"operator",
		`{"reason":"x"}`,
	); w.Code !=
		http.StatusBadRequest {
		t.Fatalf("redrive with a short reason = %d %s, want 400", w.Code, w.Body)
	}
	api := adapters.HTTP{
		Redrive: f.redriveHandler(f.bus.Conn), Discard: app.NewDiscardDeadLetterHandler(f.uow, f.clock),
	}
	for _, ctx := range []context.Context{
		t.Context(),
		auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: "not-a-uuid", Role: "operator"}),
	} {
		body := &adminapi.RedriveDeadLetterJSONRequestBody{Reason: "posthog fixed"}
		_, err := api.RedriveDeadLetter(ctx, adminapi.RedriveDeadLetterRequestObject{Id: f.ids.NewV7(), Body: body})
		if errs.CodeOf(err) != errs.CodeAdminForbidden {
			t.Fatalf("RedriveDeadLetter = %v, want admin_forbidden", err)
		}
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters RENAME TO dead_letters_gone`); err != nil {
		t.Fatal(err)
	}
	if w := adminRequest(t, h, "/v1/admin/dead-letters", "viewer"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("list without the table = %d %s, want 503", w.Code, w.Body)
	}
}
