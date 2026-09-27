package fakes_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func upstreams() []string {
	return []string{"privy", "jupiter", "rpc", "helius", "xstocks", "apns", "ably"}
}

type inProcess struct{ h http.Handler }

func (p inProcess) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	p.h.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	return rec.Result(), nil
}

type reply struct {
	status int
	header http.Header
	body   string
}

func call(ctx context.Context, t *testing.T, c *httpclient.Client, method, path, body string) (reply, error) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rd)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return reply{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: string(b)}, nil
}

func mustCall(ctx context.Context, t *testing.T, c *httpclient.Client, method, path, body string) reply {
	t.Helper()
	got, err := call(ctx, t, c, method, path, body)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func overHTTP(t *testing.T, timeout time.Duration) *httpclient.Client {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	return httpclient.New("fakes", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(timeout))
}

func inProc(opts ...httpclient.Option) *httpclient.Client {
	return httpclient.New("fakes", append([]httpclient.Option{
		httpclient.WithBaseURL("http://fakes.test"),
		httpclient.WithTimeout(time.Hour),
		httpclient.WithTransport(inProcess{fakes.New()}),
	}, opts...)...)
}

func script(ctx context.Context, t *testing.T, c *httpclient.Client, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustCall(ctx, t, c, http.MethodPost, "/_script", string(raw)); got.status != http.StatusNoContent {
		t.Fatalf("POST /_script %s = %d %q, want 204", raw, got.status, got.body)
	}
}

func TestReplay_servesEachUpstreamHealthFixture(t *testing.T) {
	t.Parallel()
	c := overHTTP(t, 10*time.Second)
	for _, u := range upstreams() {
		got := mustCall(t.Context(), t, c, http.MethodGet, "/"+u+"/_health", "")
		want := `{"status": "ok", "upstream": "` + u + `"}`
		if got.status != http.StatusOK || got.body != want || got.header.Get("Content-Type") != "application/json" {
			t.Fatalf("GET /%s/_health = %d %q %v, want 200 %q as JSON", u, got.status, got.body, got.header, want)
		}
	}
}

func TestReplay_routeWithoutFixtureIs501AndUnknownUpstreamIs404(t *testing.T) {
	t.Parallel()
	c := overHTTP(t, 10*time.Second)
	if got := mustCall(t.Context(), t, c, http.MethodPost, "/privy/api/v1/users", "{}"); got.status != 501 ||
		!strings.Contains(got.body, "/privy/api/v1/users") {
		t.Fatalf("unrecorded route = %d %q, want 501 naming the route", got.status, got.body)
	}
	if got := mustCall(t.Context(), t, c, http.MethodGet, "/stripe/_health", ""); got.status != http.StatusNotFound {
		t.Fatalf("unknown upstream = %d, want 404", got.status)
	}
}

func TestScript_failsTheNextNCallsThenReplaysAgain(t *testing.T) {
	t.Parallel()
	c := overHTTP(t, 10*time.Second)
	script(t.Context(), t, c, fakes.Step{
		Route: "/jupiter/_health", Action: fakes.ActionFail, Status: http.StatusNotFound,
		Body: json.RawMessage(`{"error":"gone"}`), Times: 2,
	})
	script(t.Context(), t, c, fakes.Step{Route: "/jupiter/_health", Action: fakes.ActionSucceed})
	script(t.Context(), t, c, fakes.Step{Route: "/jupiter/_health", Action: fakes.ActionFail, Status: 418})

	got := make([]int, 0, 5)
	for range 5 {
		r := mustCall(t.Context(), t, c, http.MethodGet, "/jupiter/_health", "")
		got = append(got, r.status)
		if r.status == http.StatusNotFound &&
			(r.body != `{"error":"gone"}` || r.header.Get("Content-Type") != "application/json") {
			t.Fatalf("scripted failure body = %q", r.body)
		}
	}
	want := []int{404, 404, 200, 418, 200}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statuses = %v, want %v", got, want)
		}
	}
	if other := mustCall(t.Context(), t, c, http.MethodGet, "/privy/_health", ""); other.status != http.StatusOK {
		t.Fatalf("unscripted route = %d, want its fixture", other.status)
	}
}

func TestScript_rejectsInvalidSteps(t *testing.T) {
	t.Parallel()
	c := overHTTP(t, 10*time.Second)
	for body, field := range map[string]string{
		`{"route":"/privy/_health","action":"explode"}`:             "action",
		`{"route":"/privy/_health","action":"fail"}`:                "status",
		`{"route":"/privy/_health","action":"fail","status":200}`:   "status",
		`{"route":"/privy/_health","action":"fail","status":600}`:   "status",
		`{"route":"/privy/_health","action":"delay"}`:               "delay",
		`{"route":"/privy/_health","action":"delay","delay":"-1s"}`: "delay",
		`{"route":"/privy/_health","action":"hang","times":-1}`:     "times",
		`{"route":"/stripe/x","action":"hang"}`:                     "route",
		`{"route":"privy/_health","action":"hang"}`:                 "route",
		`{"route":"/privy","action":"hang"}`:                        "route",
		`{"route":"/privy/_health","action":"hang","extra":1}`:      "decode",
	} {
		got := mustCall(t.Context(), t, c, http.MethodPost, "/_script", body)
		if got.status != http.StatusBadRequest || !strings.Contains(got.body, field) {
			t.Fatalf("POST /_script %s = %d %q, want 400 naming %s", body, got.status, got.body, field)
		}
	}
}

func TestScript_delayHoldsTheReplayInFakeTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := inProc()
		script(t.Context(), t, c, fakes.Step{Route: "/helius/_health", Action: fakes.ActionDelay, Delay: "3s"})
		start := now()

		got := mustCall(t.Context(), t, c, http.MethodGet, "/helius/_health", "")
		if took := now().Sub(start); got.status != http.StatusOK || took != 3*time.Second {
			t.Fatalf("delayed replay = %d after %v, want 200 after 3s", got.status, took)
		}
	})
}

func TestScript_delayedCallCancelledByTheClientDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := inProc(httpclient.WithTimeout(time.Second))
		script(t.Context(), t, c, fakes.Step{Route: "/xstocks/_health", Action: fakes.ActionDelay, Delay: "1m"})

		_, err := call(t.Context(), t, c, http.MethodGet, "/xstocks/_health", "")
		if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
			t.Fatalf("err = %v, want upstream_timeout", err)
		}
	})
}

func TestScript_hangThenCancelReturnsPromptlyWithoutLeaks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := inProc()
		script(t.Context(), t, c, fakes.Step{Route: "/apns/_health", Action: fakes.ActionHang})
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(2*time.Second, cancel)
		start := now()

		_, err := call(ctx, t, c, http.MethodGet, "/apns/_health", "")
		if !errors.Is(err, context.Canceled) || now().Sub(start) != 2*time.Second {
			t.Fatalf("err = %v after %v, want context.Canceled at 2s", err, now().Sub(start))
		}
		if got := mustCall(t.Context(), t, c, http.MethodGet, "/apns/_health", ""); got.status != http.StatusOK {
			t.Fatalf("after the hang = %d, want the fixture again", got.status)
		}
	})
}

func TestScript_hangOverRealHTTPReleasesWhenTheClientTimesOut(t *testing.T) {
	t.Parallel()
	c := overHTTP(t, 50*time.Millisecond)
	script(t.Context(), t, c, fakes.Step{Route: "/ably/_health", Action: fakes.ActionHang})

	_, err := call(t.Context(), t, c, http.MethodGet, "/ably/_health", "")
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("err = %v, want upstream_timeout", err)
	}
	if got := mustCall(t.Context(), t, c, http.MethodGet, "/ably/_health", ""); got.status != http.StatusOK {
		t.Fatalf("after the hang = %d, want the fixture again", got.status)
	}
}

func now() time.Time { return clock.Real{}.Now() }

func TestNew_panicsOnAFixtureItCannotLoad(t *testing.T) {
	t.Parallel()
	for name, fsys := range map[string]fstest.MapFS{
		"bad json":     {"fx/privy/_health.json": {Data: []byte(`{"status":`)}},
		"missing root": {"other/privy/_health.json": {Data: []byte(`{"status":200}`)}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: New did not panic", name)
				}
			}()
			fakes.NewFrom(fsys, "fx")
		}()
	}
}

func TestNew_servesFixturesFromTheGivenFS(t *testing.T) {
	t.Parallel()
	srv := fakes.NewFrom(fstest.MapFS{"fx/rpc/getSlot.json": {Data: []byte(`{"status":200,"body":{"slot":7}}`)}}, "fx")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/rpc/getSlot", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"slot":7}` {
		t.Fatalf("GET /rpc/getSlot = %d %q", rec.Code, rec.Body.String())
	}
}
