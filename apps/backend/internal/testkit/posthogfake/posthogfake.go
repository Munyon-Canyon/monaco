package posthogfake

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const batchRoute = "/posthog/batch/"

type Fake struct {
	srv   *httptest.Server
	fakes *fakes.Server
}

func New(t *testing.T) *Fake {
	t.Helper()
	f := fakes.New()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &Fake{srv: srv, fakes: f}
}

func (p *Fake) Host() string { return p.srv.URL + "/posthog" }

func (p *Fake) Captures() []fakes.PostHogCapture { return p.fakes.PostHogCaptures() }

func (p *Fake) Received() int { return p.fakes.PostHogReceived() }

func (p *Fake) Fail(t *testing.T, status, times int) {
	t.Helper()
	p.Script(t, fakes.Step{Route: batchRoute, Action: fakes.ActionFail, Status: status, Times: times})
}

func (p *Fake) Script(t *testing.T, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("posthogfake.Fake.Script: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, p.srv.URL+"/_script", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("posthogfake.Fake.Script: %v", err)
	}
	resp, err := p.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("posthogfake.Fake.Script: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("posthogfake.Fake.Script: POST /_script %s answered %d, want 204", raw, resp.StatusCode)
	}
}
