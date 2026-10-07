package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func queueBody(t *testing.T, f lookupFixture, path string, body any) {
	t.Helper()
	w := adminRequest(t, f.h, path, "viewer")
	if err := json.Unmarshal(w.Body.Bytes(), body); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s (%v)", path, w.Code, w.Body, err)
	}
}

func TestAdminQueue_StuckTxns(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	stale := f.swap(t, c.ID.UUID(), "submitted", txnSignature, "req-1")
	f.clock.Advance(10 * time.Minute)
	f.swap(t, c.ID.UUID(), "submitted", "sig-fresh", "req-2")
	f.swap(t, c.ID.UUID(), "confirmed", "sig-done", "req-3")
	var body adminapi.StuckTxns
	queueBody(t, f, "/v1/admin/queues/stuck-txns?older_than=5m", &body)
	if len(body.Items) != 1 || body.Items[0].Id != stale || body.Items[0].Status != "submitted" ||
		body.Items[0].AgeSeconds != 600 || *body.Items[0].TxSignature != txnSignature ||
		body.Items[0].CabalId != c.ID.UUID() || body.Items[0].Symbol != "AAPLx" || body.Items[0].Action != "buy" {
		t.Fatalf("items = %+v", body.Items)
	}
}

func TestAdminQueue_StuckTxns_DefaultsToFiveMinutesAndHonoursTheLimit(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	f.swap(t, c.ID.UUID(), "created", "", "")
	f.swap(t, c.ID.UUID(), "created", "", "")
	f.clock.Advance(6 * time.Minute)
	var body adminapi.StuckTxns
	queueBody(t, f, "/v1/admin/queues/stuck-txns", &body)
	if len(body.Items) != 2 ||
		body.Items[0].TxSignature != nil {
		t.Fatalf("default items = %+v", body.Items)
	}
	var limited, tooNew adminapi.StuckTxns
	queueBody(t, f, "/v1/admin/queues/stuck-txns?limit=1", &limited)
	queueBody(t, f, "/v1/admin/queues/stuck-txns?older_than=1h", &tooNew)
	if len(limited.Items) != 1 || len(tooNew.Items) != 0 {
		t.Fatalf("limited = %+v, older_than=1h = %+v", limited.Items, tooNew.Items)
	}
}

func TestAdminQueue_OlderThanMustBeADurationUpToADay(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	for _, query := range []string{"older_than=soon", "older_than=0s", "older_than=-5m", "older_than=25h"} {
		for _, route := range []string{"stuck-txns", "unpublished-events"} {
			path := "/v1/admin/queues/" + route + "?" + query
			w := adminRequest(t, f.h, path, "viewer")
			var problem map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || w.Code != http.StatusBadRequest ||
				problem["code"] != string(errs.CodeInvalidInput) {
				t.Errorf("GET %s = %d %s", path, w.Code, w.Body)
			}
		}
	}
}

func TestAdminQueue_UnpublishedEvents(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	aggregate := f.ids.NewV7()
	f.swapEvent(t, aggregate, "trade.submitted", f.clock.Now().Add(-time.Minute))
	f.swapEvent(t, aggregate, "trade.confirmed", f.clock.Now().Add(-5*time.Second))
	f.exec(t, `INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at,
		published_at) VALUES ($1, 'swap', $2, 'trade.failed', '{"v":1}', 'system', 'test', $3, $3)`,
		f.ids.NewV7(), aggregate, f.clock.Now().Add(-time.Hour))
	var body adminapi.UnpublishedEvents
	queueBody(t, f, "/v1/admin/queues/unpublished-events", &body)
	if len(body.Items) != 1 || body.Items[0].Type != "trade.submitted" || body.Items[0].AgeSeconds != 60 ||
		body.Items[0].Aggregate.Type != "swap" || body.Items[0].Aggregate.Id != aggregate {
		t.Fatalf("items = %+v", body.Items)
	}
	var limited adminapi.UnpublishedEvents
	queueBody(t, f, "/v1/admin/queues/unpublished-events?older_than=1s&limit=1", &limited)
	if len(limited.Items) != 1 {
		t.Fatalf("limited items = %+v", limited.Items)
	}
}

func TestAdminQueue_CountsEveryQueueWithItsRoute(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	f.swap(t, c.ID.UUID(), "created", "", "")
	f.swapEvent(t, f.ids.NewV7(), "trade.submitted", f.clock.Now())
	f.exec(t, `INSERT INTO dead_letters (id, stream_seq, consumer, handler, subject, code, error, letter, status,
		first_seen_at, last_seen_at) VALUES ($1, 1, 'c', 'h', 's', 'c', 'e', '{}', 'open', $2, $2),
		($3, 2, 'c', 'h', 's', 'c', 'e', '{}', 'resolved', $2, $2)`, f.ids.NewV7(), f.clock.Now(), f.ids.NewV7())
	f.clock.Advance(time.Hour)
	var body adminapi.AdminQueues
	queueBody(t, f, "/v1/admin/queues", &body)
	want := []adminapi.AdminQueue{
		{Name: "stuck_txns", Count: 1, Href: "/v1/admin/queues/stuck-txns"},
		{Name: "unpublished_events", Count: 1, Href: "/v1/admin/queues/unpublished-events"},
		{Name: "dead_letters", Count: 1, Href: "/v1/admin/dead-letters"},
	}
	if len(body.Queues) != len(want) {
		t.Fatalf("queues = %+v", body.Queues)
	}
	for i, q := range want {
		if body.Queues[i] != q {
			t.Errorf("queue %d = %+v, want %+v", i, body.Queues[i], q)
		}
	}
}

func TestAdminQueue_QueryCount(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	f.swap(t, c.ID.UUID(), "created", "", "")
	f.swapEvent(t, f.ids.NewV7(), "trade.submitted", f.clock.Now())
	f.clock.Advance(time.Hour)
	for name, path := range map[string]string{
		"admin GetAdminQueues":       "/v1/admin/queues",
		"admin GetStuckTxns":         "/v1/admin/queues/stuck-txns",
		"admin GetUnpublishedEvents": "/v1/admin/queues/unpublished-events",
	} {
		testkit.AssertQueries(t, name, func() {
			if w := adminRequest(t, f.h, path, "viewer"); w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d %s", path, w.Code, w.Body)
			}
		})
	}
}

func TestDeadLetterCount_ReportsDBUnavailableWhenTheDatabaseIsDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (adapters.DeadLetterCount{DB: testkit.DB(t)}).CountOpen(ctx); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("CountOpen = %v", err)
	}
}

func TestQueueAdapters_PassPortFailuresOn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	calls := []struct {
		method string
		call   func(h adapters.HTTP) error
	}{
		{"CountStuck", func(h adapters.HTTP) error {
			_, err := h.GetAdminQueues(ctx, adminapi.GetAdminQueuesRequestObject{})
			return err
		}},
		{"Stuck", func(h adapters.HTTP) error {
			_, err := h.GetStuckTxns(ctx, adminapi.GetStuckTxnsRequestObject{})
			return err
		}},
		{"Unpublished", func(h adapters.HTTP) error {
			_, err := h.GetUnpublishedEvents(ctx, adminapi.GetUnpublishedEventsRequestObject{})
			return err
		}},
	}
	for _, c := range calls {
		if err := c.call(adapters.HTTP{Queues: queues(newStub(failing(c.method)))}); !isPortDown(err) {
			t.Errorf("%s down = %v", c.method, err)
		}
	}
}
