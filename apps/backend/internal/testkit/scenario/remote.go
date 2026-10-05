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
	URL           string
	FakesURL      string
	PrivyAppID    string
	ClientIP      string
	Flow          string
	Trigger       string
	Pool          *pgxpool.Pool
	Bus           *bus.Conn
	Consumers     []bus.Consumer
	Mint          func(userID string) string
	Converge      func(ctx context.Context, eventIDs []string) error
	Crash         func(ctx context.Context, point faultpoint.Name) error
	Restart       func(ctx context.Context) error
	Enter         func(stage Stage)
	Exchanged     func(e Exchange)
	Logs          func(from int) (lines []string, changed <-chan struct{})
	CoreSubscribe func(t T, subject string) <-chan []byte
}

func Against(ctx context.Context, t T, r Remote) *Scenario {
	rm := &remote{Remote: r}
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	t.Cleanup(client.CloseIdleConnections)
	b := &backend{
		baseURL:       r.URL,
		clientIP:      r.ClientIP,
		flow:          r.Flow,
		trigger:       r.Trigger,
		client:        client,
		note:          newNotifier(),
		pool:          r.Pool,
		bus:           r.Bus,
		privyToken:    func(sub string) string { return fakes.PrivyAccessToken(r.PrivyAppID, sub, time.Now(), time.Hour) },
		control:       rm.controlFakes(client),
		mint:          func(id ids.UserID) string { return r.Mint(id.String()) },
		newUserID:     func() (ids.UserID, error) { return ids.ParseUserID(ids.Real{}.NewV7().String()) },
		enter:         r.Enter,
		exchanged:     r.Exchanged,
		events:        rm.events,
		eventPayloads: rm.eventPayloads,
		awaitHandled:  rm.awaitHandled,
		published:     rm.published,
		hold:          func() {},
		crashAt:       rm.crashAt,
		seed:          rm.seed,
		lines:         r.Logs,
		tick:          func(T, string) func() { return func() {} },
		coreSubscribe: r.CoreSubscribe,
		tickCrash:     rm.tickCrash,
	}
	if r.Restart != nil {
		b.restart = func(t T) {
			if err := r.Restart(ctx); err != nil {
				t.Fatalf("scenario: restart after faultpoint: %v", err)
			}
		}
	}
	return newScenario(t, b)
}

type remote struct {
	Remote
}

func (r *remote) controlFakes(client *http.Client) func(ctx context.Context, t T, path string, body any) {
	return func(ctx context.Context, t T, path string, body any) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("scenario: fakes %s %+v: %v", path, body, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.FakesURL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("scenario: fakes %s %s: %v", path, raw, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("scenario: fakes %s %s: %v", path, raw, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if answer, _ := io.ReadAll(resp.Body); resp.StatusCode != http.StatusNoContent {
			t.Fatalf("scenario: fakes %s %s answered %d %s", path, raw, resp.StatusCode, answer)
		}
	}
}

func (r *remote) events(t T, typ events.Type, actors []string) []string {
	t.Helper()
	rows, err := r.Pool.Query(t.Context(),
		`SELECT id::text FROM events WHERE type = $1 AND actor_id = ANY($2) ORDER BY id`, string(typ), actors)
	return scanIDs(t, typ, rows, err)
}

func (r *remote) eventPayloads(t T, typ events.Type) [][]byte {
	t.Helper()
	rows, err := r.Pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(typ))
	if err != nil {
		t.Fatalf("scenario: read %s event payloads: %v", typ, err)
	}
	defer rows.Close()
	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scenario: scan %s event payload: %v", typ, err)
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("scenario: read %s event payloads: %v", typ, err)
	}
	return payloads
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

func (r *remote) tickCrash(t T, _ string, point faultpoint.Name) {
	t.Helper()
	r.crashAt(t, point)
}

func (r *remote) seed(t T, name string) []testkit.Seeded {
	t.Helper()
	return testkit.Seed(t, r.Pool, name, r.Consumers...)
}
