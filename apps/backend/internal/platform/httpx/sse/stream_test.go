package sse_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type routes struct {
	httpx.Health
	sse.Stream
	httpx.GovernanceRoutes
	httpx.IdentityRoutes
	httpx.NotifyRoutes
	httpx.ReferralsRoutes
	httpx.SocialRoutes
	httpx.SystemRoutes
	httpx.CabalRoutes
	httpx.CabalJoinRoutes
	httpx.CabalAccessRoutes
	httpx.CabalPictureRoutes
	httpx.MarketRoutes
	httpx.TreasuryRoutes
}

type unusedStore struct{ httpx.IdempotencyStore }

type server struct {
	*httptest.Server
	verifier *auth.DevVerifier
}

func newServer(t *testing.T, f *fixture, timeouts config.Timeouts) server {
	t.Helper()
	verifier, err := auth.NewDevVerifier(config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "k"}},
		clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{}, io.Discard), Tracer: noop.NewTracerProvider(),
		Clock: clock.Real{}, IDs: ids.Real{}, MaxBodyBytes: 1 << 10,
		Idempotency: unusedStore{}, Verifier: verifier,
	}, routes{Stream: sse.NewStream(f.hub, clock.Real{})}, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Config = httpx.NewServer(h, timeouts)
	srv.Start()
	t.Cleanup(srv.Close)
	transport, ok := srv.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("test client transport is %T", srv.Client().Transport)
	}
	transport.ResponseHeaderTimeout = 5 * time.Second
	return server{Server: srv, verifier: verifier}
}

type opened struct {
	status      int
	contentType string
	events      <-chan string
}

func (s server) open(ctx context.Context, t *testing.T, user ids.UserID, header http.Header) opened {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if !user.IsZero() {
		req.Header.Set("Authorization", "Bearer "+s.verifier.Mint(user.String(), clock.Real{}.Now().Add(time.Hour)))
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan string, sse.Buffer)
	done := make(chan struct{})
	go readEvents(resp, out, done)
	t.Cleanup(func() {
		_ = resp.Body.Close()
		<-done
	})
	return opened{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), events: out}
}

func readEvents(resp *http.Response, out chan<- string, done chan<- struct{}) {
	defer close(done)
	defer close(out)
	defer func() { _ = resp.Body.Close() }()
	r := bufio.NewReader(resp.Body)
	var event []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if line = strings.TrimSuffix(line, "\n"); line != "" {
			event = append(event, line)
			continue
		}
		out <- strings.Join(event, "\n")
		event = nil
	}
}

func (o opened) stream(t *testing.T) <-chan string {
	t.Helper()
	if o.status != http.StatusOK || o.contentType != "text/event-stream" {
		t.Fatalf("GET /v1/stream = %d %s, want 200 text/event-stream", o.status, o.contentType)
	}
	return o.events
}

func nextEvent(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("stream ended, want an event")
		}
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}
	return ""
}

func hintEvent(id, key, what string) string {
	return "id: " + id + "\nevent: hint\ndata: {\"key\":\"" + key + "\",\"what\":\"" + what + "\"}"
}

func TestStream_deliversOnlyTheHintsTheCallerMayRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	srv := newServer(t, f, config.Timeouts{HTTPServerRead: time.Minute, HTTPServerWrite: time.Minute})
	phone, fortyTwo := f.user(t), f.cabal(t)
	stream := srv.open(t.Context(), t, phone, nil).stream(t)

	f.deliver(t, cabalHint(fortyTwo), userHint(f.user(t), "balance"), userHint(phone, "balance"), "global.feed")

	if got, want := nextEvent(t, stream), hintEvent("1", "user:"+phone.String(), "balance"); got != want {
		t.Fatalf("first event = %q, want %q", got, want)
	}
	if got, want := nextEvent(t, stream), hintEvent("2", "global", "feed"); got != want {
		t.Fatalf("second event = %q, want %q", got, want)
	}
}

func TestStream_lastEventIDGetsOneResyncFirst(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	srv := newServer(t, f, config.Timeouts{HTTPServerRead: time.Minute, HTTPServerWrite: time.Minute})
	phone := f.user(t)
	stream := srv.open(t.Context(), t, phone, http.Header{"Last-Event-Id": {"7"}}).stream(t)

	f.deliver(t, userHint(phone, "balance"))

	if got := nextEvent(t, stream); got != "event: resync\ndata: {}" {
		t.Fatalf("first event = %q, want one resync", got)
	}
	if got, want := nextEvent(t, stream), hintEvent("1", "user:"+phone.String(), "balance"); got != want {
		t.Fatalf("event after resync = %q, want %q", got, want)
	}
}

func TestStream_requiresABearerToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	srv := newServer(t, f, config.Timeouts{HTTPServerRead: time.Minute, HTTPServerWrite: time.Minute})
	resp := srv.open(t.Context(), t, ids.UserID{}, nil)
	if resp.status != http.StatusUnauthorized || resp.contentType != "application/problem+json" {
		t.Fatalf("GET /v1/stream without a token = %d %s, want 401 problem", resp.status, resp.contentType)
	}
}

func TestStream_outlivesTheServerTimeouts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	const timeout = 100 * time.Millisecond
	srv := newServer(t, f, config.Timeouts{HTTPServerRead: timeout, HTTPServerWrite: timeout})
	phone := f.user(t)
	stream := srv.open(t.Context(), t, phone, nil).stream(t)

	select {
	case e := <-stream:
		t.Fatalf("stream sent %q or ended within 3x the server timeouts, want it idle and open", e)
	case <-time.After(3 * timeout):
	}
	f.deliver(t, userHint(phone, "balance"))

	if got, want := nextEvent(t, stream), hintEvent("1", "user:"+phone.String(), "balance"); got != want {
		t.Fatalf("event after 3x the server timeouts = %q, want %q", got, want)
	}
}

func TestStream_disconnectFreesTheSubscriberAcrossManyCycles(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	srv := newServer(t, f, config.Timeouts{HTTPServerRead: time.Minute, HTTPServerWrite: time.Minute})
	phone := f.user(t)
	for range 1000 {
		ctx, cancel := context.WithCancel(t.Context())
		srv.open(ctx, t, phone, nil).stream(t)
		cancel()
	}
	srv.Close()

	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections after 1000 connect/disconnect cycles = %d, want 0", n)
	}
}

type recorder struct {
	mu     sync.Mutex
	header http.Header
	body   bytes.Buffer
	fail   bool
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(int) {}

func (r *recorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return 0, errs.New(errs.CodeClientClosed, "test.recorder.Write")
	}
	return r.body.Write(b)
}

func (r *recorder) Flush() {}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

type deadlineFails struct{ *recorder }

func (deadlineFails) SetWriteDeadline(time.Time) error {
	return errs.New(errs.CodeClientClosed, "test.SetWriteDeadline")
}

func visit(ctx context.Context, t *testing.T, f *fixture, w http.ResponseWriter) <-chan error {
	t.Helper()
	actor := auth.Actor{Kind: auth.ActorUser, ID: f.user(t).String()}
	resp, err := sse.NewStream(f.hub, clock.Real{}).GetStream(auth.WithActor(ctx, actor), api.GetStreamRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- resp.VisitGetStreamResponse(w) }()
	return done
}

func TestStream_heartbeatEvery15Seconds(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		w := &recorder{header: http.Header{}}
		done := visit(ctx, t, f, w)

		for i, step := range []struct {
			wait time.Duration
			want string
		}{
			{sse.Heartbeat - time.Second, ""},
			{time.Second, ": heartbeat\n\n"},
			{sse.Heartbeat, ": heartbeat\n\n: heartbeat\n\n"},
		} {
			<-time.After(step.wait)
			synctest.Wait()
			if got := w.String(); got != step.want {
				t.Fatalf("step %d: body = %q, want %q", i, got, step.want)
			}
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Visit = %v, want nil after the client left", err)
		}
		if n := f.sum(t, "monaco_sse_connections"); n != 0 {
			t.Fatalf("connections = %d, want 0", n)
		}
	})
}

func TestStream_endsWhenTheHubStops(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		done := visit(t.Context(), t, f, &recorder{header: http.Header{}})
		synctest.Wait()
		f.stop()
		if err := <-done; err != nil {
			t.Fatalf("Visit = %v, want nil when the hub stops", err)
		}
	})
}

func TestStream_endsOnAFailedWriteOrDeadline(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for name, w := range map[string]http.ResponseWriter{
		"write":    &recorder{header: http.Header{}, fail: true},
		"deadline": deadlineFails{&recorder{header: http.Header{}}},
	} {
		done := visit(t.Context(), t, f, w)
		f.deliver(t, "global.feed")
		if err := waitVisit(t, name, done); err != nil {
			t.Fatalf("%s: Visit = %v, want nil", name, err)
		}
	}
	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections = %d, want 0", n)
	}
}

func TestStream_refusesCallersTheHubRejects(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	stream := sse.NewStream(f.hub, clock.Real{})
	if _, err := stream.GetStream(
		t.Context(),
		api.GetStreamRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("GetStream without an actor = %v, want unauthorized", err)
	}
	agent := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAgent, ID: f.user(t).String()})
	if _, err := stream.GetStream(agent, api.GetStreamRequestObject{}); errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("GetStream as an agent = %v, want forbidden", err)
	}
}

func waitVisit(t *testing.T, name string, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: Visit still serving after 5s", name)
		return nil
	}
}
