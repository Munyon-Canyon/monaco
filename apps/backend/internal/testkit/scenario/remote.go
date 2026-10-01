package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type Remote struct {
	URL        string
	FakesURL   string
	PrivyAppID string
	Pool       *pgxpool.Pool
	Consumers  []bus.Consumer
	Mint       func(userID string) string
	Converge   func(ctx context.Context, eventIDs []string) error
	Crash      func(ctx context.Context, point faultpoint.Name) error
	Enter      func(stage Stage)
	Exchanged  func(e Exchange)
	Logs       func(from int) (lines []string, changed <-chan struct{})
}

func Against(t T, r Remote) *Scenario {
	rm := &remote{Remote: r}
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	t.Cleanup(client.CloseIdleConnections)
	return newScenario(t, &backend{
		baseURL: r.URL, client: client, note: newNotifier(), pool: r.Pool,
		privyToken: func(sub string) string { return fakes.PrivyAccessToken(r.PrivyAppID, sub, time.Now(), time.Hour) },
		script:     rm.scriptFakes(client),
		mint:       func(id ids.UserID) string { return r.Mint(id.String()) },
		newUserID:  func() (ids.UserID, error) { return ids.ParseUserID(ids.Real{}.NewV7().String()) },
		enter:      r.Enter, exchanged: r.Exchanged, events: rm.events, awaitHandled: rm.awaitHandled,
		published: rm.published, hold: func() {}, crashAt: rm.crashAt, seed: rm.seed, lines: r.Logs,
		tick: func(T, string) func() { return func() {} },
	})
}

type remote struct {
	Remote
}

func (r *remote) scriptFakes(client *http.Client) func(ctx context.Context, t T, step fakes.Step) {
	return func(ctx context.Context, t T, step fakes.Step) {
		t.Helper()
		raw, err := json.Marshal(step)
		if err != nil {
			t.Fatalf("scenario: script %+v: %v", step, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.FakesURL+"/_script", bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("scenario: script %s: %v", raw, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("scenario: script %s: %v", raw, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if body, _ := io.ReadAll(resp.Body); resp.StatusCode != http.StatusNoContent {
			t.Fatalf("scenario: script %s answered %d %s", raw, resp.StatusCode, body)
		}
	}
}

func (r *remote) events(t T, typ events.Type, actors []string) []string {
	t.Helper()
	rows, err := r.Pool.Query(t.Context(),
		`SELECT id::text FROM events WHERE type = $1 AND actor_id = ANY($2) ORDER BY id`, string(typ), actors)
	return scanIDs(t, typ, rows, err)
}

func (r *remote) awaitHandled(t T, typ events.Type, eventIDs []string) {
	t.Helper()
	if err := r.Converge(t.Context(), eventIDs); err != nil {
		t.Fatalf("scenario: every consumer of %s acking %v: %v", typ, eventIDs, err)
	}
}

func (r *remote) published(t T, typ events.Type, eventIDs []string) uint64 {
	t.Helper()
	var n uint64
	err := r.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM events WHERE id::text = ANY($1) AND published_at IS NOT NULL`, eventIDs).Scan(&n)
	if err != nil {
		t.Fatalf("scenario: count published %s events: %v", typ, err)
	}
	return n
}

func (r *remote) crashAt(t T, point faultpoint.Name) {
	t.Helper()
	if err := r.Crash(t.Context(), point); err != nil {
		t.Fatalf("scenario: crash at %s: %v", point, err)
	}
}

func (r *remote) seed(t T, name string) []testkit.Seeded {
	t.Helper()
	return testkit.Seed(t, r.Pool, name, r.Consumers...)
}
