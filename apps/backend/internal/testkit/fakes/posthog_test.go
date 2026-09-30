package fakes_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	batchPath = "/posthog/batch/"
	firstID   = "0190a5d0-0000-7000-8000-000000000001"
	secondID  = "0190a5d0-0000-7000-8000-000000000002"
)

func postHog(t *testing.T) (*fakes.Server, *httpclient.Client) {
	t.Helper()
	srv := fakes.New()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, httpclient.New("fakes", httpclient.WithBaseURL(ts.URL), httpclient.WithTimeout(time.Minute))
}

func stamp() time.Time { return clock.Real{}.Now().UTC().Truncate(time.Second) }

func batch(events ...string) string {
	return `{"api_key":"k","batch":[` + strings.Join(events, ",") + `]}`
}

func event(id string, when time.Time, extra string) string {
	return `{"uuid":"` + id + `","event":"probe_fired","distinct_id":"u1","timestamp":"` +
		when.Format(time.RFC3339) + `"` + extra + `}`
}

func TestPostHogBatch_recordsTheCapturesAndServesTheOkFixture(t *testing.T) {
	t.Parallel()
	srv, c := postHog(t)
	when := stamp()
	body := batch(
		event(firstID, when, `,"properties":{"cabal":"alpha","$set":{"tier":"gold"}}`),
		event(secondID, when, ``),
	)
	got := mustCall(t.Context(), t, c, http.MethodPost, batchPath, body)
	if got.status != http.StatusOK || got.body != `{"status": 1}` ||
		got.header.Get("Content-Type") != "application/json" {
		t.Fatalf(
			"POST %s = %d %q %v, want 200 %q as JSON",
			batchPath,
			got.status,
			got.body,
			got.header,
			`{"status": 1}`,
		)
	}
	ids := []string{firstID, secondID}
	want := []fakes.PostHogCapture{
		{
			APIKey: "k", Event: "probe_fired", DistinctID: "u1", Timestamp: when,
			Properties: map[string]any{"cabal": "alpha"}, Set: map[string]any{"tier": "gold"},
		},
		{APIKey: "k", Event: "probe_fired", DistinctID: "u1", Timestamp: when},
	}
	captures := srv.PostHogCaptures()
	if len(captures) != len(want) {
		t.Fatalf("captures = %+v, want %d", captures, len(want))
	}
	for i := range captures {
		captured := captures[i]
		if captured.UUID.String() != ids[i] {
			t.Errorf("capture %d has uuid %s, want %s", i, captured.UUID, ids[i])
		}
		captured.UUID = want[i].UUID
		if !reflect.DeepEqual(captured, want[i]) {
			t.Errorf("capture %d = %+v, want %+v", i, captured, want[i])
		}
	}
}

func TestPostHogBatch_rejectsWhatPostHogWouldDropOrRefuse(t *testing.T) {
	t.Parallel()
	when := stamp()
	at := `"` + when.Format(time.RFC3339) + `"`
	tests := map[string]struct {
		body   string
		status int
	}{
		"not json":      {`{`, http.StatusBadRequest},
		"unknown field": {`{"api_key":"k","batch":[],"extra":1}`, http.StatusBadRequest},
		"no api key": {
			`{"batch":[{"uuid":"` + firstID + `","event":"e","distinct_id":"u","timestamp":` + at + `}]}`,
			http.StatusUnauthorized,
		},
		"empty batch": {batch(), http.StatusBadRequest},
		"no uuid":     {batch(`{"event":"e","distinct_id":"u","timestamp":` + at + `}`), http.StatusBadRequest},
		"malformed uuid": {
			batch(`{"uuid":"nope","event":"e","distinct_id":"u","timestamp":` + at + `}`),
			http.StatusBadRequest,
		},
		"no event": {
			batch(`{"uuid":"` + firstID + `","distinct_id":"u","timestamp":` + at + `}`),
			http.StatusBadRequest,
		},
		"no distinct id": {
			batch(`{"uuid":"` + firstID + `","event":"e","timestamp":` + at + `}`),
			http.StatusBadRequest,
		},
		"no timestamp": {batch(`{"uuid":"` + firstID + `","event":"e","distinct_id":"u"}`), http.StatusBadRequest},
		"null set": {
			batch(event(firstID, when, `,"properties":{"$set":null}`)), http.StatusBadRequest,
		},
		"set that is not an object": {
			batch(event(firstID, when, `,"properties":{"$set":"x"}`)), http.StatusBadRequest,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, c := postHog(t)
			if got := mustCall(t.Context(), t, c, http.MethodPost, batchPath, tt.body); got.status != tt.status {
				t.Fatalf("POST %s %s = %d %q, want %d", batchPath, tt.body, got.status, got.body, tt.status)
			}
			if n := srv.PostHogReceived(); n != 0 {
				t.Fatalf("a refused batch left %d captures behind", n)
			}
		})
	}
}

func TestPostHogBatch_dedupesByUUIDButCountsEveryReceipt(t *testing.T) {
	t.Parallel()
	srv, c := postHog(t)
	body := batch(event(firstID, stamp(), ``))
	for range 2 {
		mustCall(t.Context(), t, c, http.MethodPost, batchPath, body)
	}
	if got, received := len(srv.PostHogCaptures()), srv.PostHogReceived(); got != 1 || received != 2 {
		t.Fatalf("captures = %d and received = %d, want 1 and 2", got, received)
	}
}

func TestPostHogBatch_aScriptedFailureRecordsNothingAndThenTheRouteRecovers(t *testing.T) {
	t.Parallel()
	srv, c := postHog(t)
	script(t.Context(), t, c,
		fakes.Step{Route: batchPath, Action: fakes.ActionFail, Status: http.StatusInternalServerError, Times: 2})
	body := batch(event(firstID, stamp(), ``))
	statuses := make([]int, 0, 3)
	for range 3 {
		statuses = append(statuses, mustCall(t.Context(), t, c, http.MethodPost, batchPath, body).status)
	}
	if !reflect.DeepEqual(statuses, []int{500, 500, 200}) || srv.PostHogReceived() != 1 {
		t.Fatalf("statuses = %v with %d received, want 500, 500, 200 and one received", statuses, srv.PostHogReceived())
	}
}
