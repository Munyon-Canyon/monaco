package httpclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type reply func(*http.Request) (*http.Response, error)

type upstream struct {
	mu      sync.Mutex
	replies []reply
	calls   []time.Time
	reqs    []*http.Request
	bodies  []string
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	body := ""
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	u.mu.Lock()
	n := len(u.calls)
	u.calls = append(u.calls, now())
	u.reqs = append(u.reqs, r)
	u.bodies = append(u.bodies, body)
	next := u.replies[min(n, len(u.replies)-1)]
	u.mu.Unlock()
	return next(r)
}

func (u *upstream) gaps() []time.Duration {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]time.Duration, 0, len(u.calls))
	for i := 1; i < len(u.calls); i++ {
		out = append(out, u.calls[i].Sub(u.calls[i-1]))
	}
	return out
}

func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.calls)
}

func status(code int, header ...string) reply {
	return func(r *http.Request) (*http.Response, error) {
		h := http.Header{}
		for i := 0; i+1 < len(header); i += 2 {
			h.Set(header[i], header[i+1])
		}
		return &http.Response{
			StatusCode: code, Status: http.StatusText(code), Header: h,
			Body: io.NopCloser(strings.NewReader("body")), Request: r,
		}, nil
	}
}

func netErr(*http.Request) (*http.Response, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.dial")
}

func hang(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func get(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func client(u *upstream, opts ...httpclient.Option) *httpclient.Client {
	return httpclient.New("privy", slices.Concat([]httpclient.Option{
		httpclient.WithBaseURL("http://privy.test/api"),
		httpclient.WithTimeout(time.Hour),
		httpclient.WithTransport(u),
	}, opts)...)
}

type result struct {
	status int
	body   string
}

func mustOK(ctx context.Context, t *testing.T, c *httpclient.Client, req *http.Request) result {
	t.Helper()
	resp, err := c.Do(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result{status: resp.StatusCode, body: string(body)}
}

func mustFail(ctx context.Context, t *testing.T, c *httpclient.Client, req *http.Request) error {
	t.Helper()
	resp, err := c.Do(ctx, req)
	if resp != nil {
		_ = resp.Body.Close()
		t.Fatalf("got %s, want an error", resp.Status)
	}
	return err
}

func errAttrs(t *testing.T, err error) (errs.Code, map[string]int64) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *errs.Error", err)
	}
	attrs := map[string]int64{}
	for _, a := range e.Attrs {
		if a.Key == "upstream" && a.Value.String() != "privy" {
			t.Fatalf("upstream attr = %q, want privy", a.Value.String())
		}
		if a.Value.Kind() == slog.KindInt64 {
			attrs[a.Key] = a.Value.Int64()
		}
	}
	return e.Code, attrs
}

func wantFailure(t *testing.T, err error, code errs.Code, status, attempt int64) {
	t.Helper()
	got, attrs := errAttrs(t, err)
	if got != code || attrs["status"] != status || attrs["attempt"] != attempt {
		t.Fatalf("err = %v with attrs %v, want code %s status %d attempt %d", err, attrs, code, status, attempt)
	}
}

func equalGaps(t *testing.T, got []time.Duration, want ...time.Duration) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("gaps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("gaps = %v, want %v", got, want)
		}
	}
}

func TestDo_retriesTheFullCappedScheduleInFakeTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusServiceUnavailable)}}
		c := client(u, httpclient.WithRetry(5, time.Second, 5*time.Second), httpclient.WithFullDelay())

		err := mustFail(t.Context(), t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamUnavailable, http.StatusServiceUnavailable, 5)
		equalGaps(t, u.gaps(), time.Second, 2*time.Second, 4*time.Second, 5*time.Second)
	})
}

func TestDo_fullJitterKeepsEachDelayWithinItsCeiling(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusBadGateway)}}
		c := client(u, httpclient.WithRetry(6, time.Second, 8*time.Second))

		err := mustFail(t.Context(), t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamUnavailable, http.StatusBadGateway, 6)
		ceilings := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
		for i, gap := range u.gaps() {
			if gap < 0 || gap > ceilings[i] {
				t.Fatalf("gap %d = %v, want within [0, %v]", i, gap, ceilings[i])
			}
		}
	})
}

func TestDo_retriesEachRetryableFailureThenReturnsTheSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{
			status(http.StatusTooManyRequests), status(http.StatusBadGateway), netErr,
			status(http.StatusGatewayTimeout), status(http.StatusOK),
		}}
		c := client(u, httpclient.WithRetry(5, time.Millisecond, time.Millisecond))

		resp := mustOK(t.Context(), t, c, get(t, "/v1/users"))
		if resp.status != http.StatusOK || resp.body != "body" || u.count() != 5 {
			t.Fatalf("got %+v after %d calls, want 200 with the body after 5", resp, u.count())
		}
	})
}

func TestDo_returnsNonRetryableStatusesWithoutRetrying(t *testing.T) {
	t.Parallel()
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		u := &upstream{replies: []reply{status(code)}}
		c := client(u, httpclient.WithRetry(5, time.Millisecond, time.Millisecond))

		resp := mustOK(t.Context(), t, c, get(t, "/v1/users"))
		if resp.status != code || u.count() != 1 {
			t.Fatalf("status %d after %d calls, want %d after 1", resp.status, u.count(), code)
		}
	}
}

func TestDo_honoursRetryAfterSecondsAndDates(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dated := func(offset time.Duration) reply {
			return func(r *http.Request) (*http.Response, error) {
				at := now().Add(offset).UTC().Format(http.TimeFormat)
				return status(http.StatusServiceUnavailable, "Retry-After", at)(r)
			}
		}
		u := &upstream{replies: []reply{
			status(http.StatusTooManyRequests, "Retry-After", "7"),
			dated(3 * time.Second),
			dated(-time.Hour),
			status(http.StatusServiceUnavailable, "Retry-After", "soon"),
			status(http.StatusNoContent),
		}}
		c := client(u, httpclient.WithRetry(5, 50*time.Millisecond, time.Minute), httpclient.WithFullDelay())

		mustOK(t.Context(), t, c, get(t, "/v1/users"))
		equalGaps(t, u.gaps(), 7*time.Second, 3*time.Second, 0, 400*time.Millisecond)
	})
}

func TestDo_breakerOpensAfterConfiguredFailuresAndHalfOpens(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{
			status(http.StatusInternalServerError), netErr, status(http.StatusServiceUnavailable),
			status(http.StatusServiceUnavailable), status(http.StatusOK),
		}}
		c := client(u, httpclient.WithBreaker(gobreaker.Settings{
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 3 },
		}))
		fail := func() error { return mustFail(t.Context(), t, c, get(t, "/v1/users")) }

		if resp := mustOK(t.Context(), t, c, get(t, "/v1/users")); resp.status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want the 500 returned to the adapter", resp.status)
		}
		wantFailure(t, fail(), errs.CodeUpstreamUnavailable, 0, 1)
		wantFailure(t, fail(), errs.CodeUpstreamUnavailable, http.StatusServiceUnavailable, 1)

		err := fail()
		wantFailure(t, err, errs.CodeUpstreamUnavailable, 0, 1)
		if !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 3 {
			t.Fatalf("err = %v after %d calls, want the open breaker to refuse without calling", err, u.count())
		}

		<-time.After(31 * time.Second)
		wantFailure(t, fail(), errs.CodeUpstreamUnavailable, http.StatusServiceUnavailable, 1)
		if err := fail(); !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("err = %v, want the failed half-open probe to reopen the breaker", err)
		}

		<-time.After(31 * time.Second)
		resp := mustOK(t.Context(), t, c, get(t, "/v1/users"))
		if resp.status != http.StatusOK || u.count() != 5 {
			t.Fatalf("status %d after %d calls, want the half-open probe to close with 200", resp.status, u.count())
		}
	})
}

func TestDo_rateLimitsDoNotOpenTheBreaker(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusTooManyRequests)}}
	c := client(u, httpclient.WithBreaker(gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 3 },
	}))
	for range 5 {
		if err := mustFail(t.Context(), t, c, get(t, "/v1/users")); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
			t.Fatalf("err = %v, want upstream_unavailable", err)
		}
	}
	if u.count() != 5 {
		t.Fatalf("calls = %d, want all five requests to reach the upstream", u.count())
	}
}

func TestDo_openBreakerIsNotRetried(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{netErr}}
		c := client(u, httpclient.WithRetry(5, time.Second, time.Second), httpclient.WithBreaker(gobreaker.Settings{
			ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 2 },
		}))

		err := mustFail(t.Context(), t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamUnavailable, 0, 3)
		if !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 2 {
			t.Fatalf("err = %v after %d calls, want the third attempt refused by the breaker", err, u.count())
		}
	})
}

func TestDo_deadlineExceededMapsToUpstreamTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{hang}}
		c := client(u, httpclient.WithTimeout(10*time.Second))
		start := now()

		err := mustFail(t.Context(), t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamTimeout, 0, 1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want it to wrap context.DeadlineExceeded", err)
		}
		if took := now().Sub(start); took != 10*time.Second {
			t.Fatalf("gave up after %v, want the configured 10s", took)
		}
	})
}

func TestDo_deadlineDuringBackoffMapsToUpstreamTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusServiceUnavailable, "Retry-After", "60")}}
		c := client(u, httpclient.WithTimeout(5*time.Second), httpclient.WithRetry(3, time.Second, time.Second))

		err := mustFail(t.Context(), t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamTimeout, http.StatusServiceUnavailable, 1)
		if u.count() != 1 {
			t.Fatalf("calls = %d, want 1", u.count())
		}
	})
}

func TestDo_cancellingAHungCallReturnsPromptlyWithoutTrippingTheBreaker(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{hang, status(http.StatusOK)}}
		c := client(u, httpclient.WithRetry(3, time.Second, time.Second), httpclient.WithBreaker(gobreaker.Settings{
			ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
		}))
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			<-time.After(time.Second)
			cancel()
		}()
		start := now()

		err := mustFail(ctx, t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamUnavailable, 0, 1)
		if !errors.Is(err, context.Canceled) || now().Sub(start) != time.Second {
			t.Fatalf("err = %v after %v, want context.Canceled at 1s", err, now().Sub(start))
		}
		if resp := mustOK(t.Context(), t, c, get(t, "/v1/users")); resp.status != http.StatusOK {
			t.Fatalf("status = %d, want the breaker still closed after a caller cancel", resp.status)
		}
	})
}

func TestDo_replaysTheBodyOnEveryAttemptAndJoinsTheBaseURL(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusBadGateway), status(http.StatusCreated)}}
	c := client(u, httpclient.WithRetry(2, 0, 0))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/wallets?chain=solana",
		bytes.NewReader([]byte(`{"owner":"u1"}`)))
	if err != nil {
		t.Fatal(err)
	}

	mustOK(t.Context(), t, c, req)
	for i, r := range u.reqs {
		if got := r.URL.String(); got != "http://privy.test/api/v1/wallets?chain=solana" {
			t.Fatalf("attempt %d url = %q", i+1, got)
		}
		if u.bodies[i] != `{"owner":"u1"}` {
			t.Fatalf("attempt %d body = %q, want the full body replayed", i+1, u.bodies[i])
		}
	}
}

func TestDo_joinsABaseURLThatHasNoPath(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	c := httpclient.New("fakes", httpclient.WithBaseURL("http://127.0.0.1:8099"), httpclient.WithTimeout(time.Second),
		httpclient.WithTransport(u))

	mustOK(t.Context(), t, c, get(t, "/privy/_health"))
	if got := u.reqs[0].URL.String(); got != "http://127.0.0.1:8099/privy/_health" {
		t.Fatalf("url = %q", got)
	}
}

func TestDo_withoutBaseURLSendsTheRequestURL(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	c := httpclient.New("rpc", httpclient.WithTimeout(time.Second), httpclient.WithTransport(u))

	mustOK(t.Context(), t, c, get(t, "https://rpc.test/v1?x=1"))
	if got := u.reqs[0].URL.String(); got != "https://rpc.test/v1?x=1" {
		t.Fatalf("url = %q", got)
	}
}

func TestDo_refusesABodyItCannotReplay(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	req := get(t, "/v1/wallets")
	req.Body = io.NopCloser(strings.NewReader("once"))

	err := mustFail(t.Context(), t, client(u), req)
	if code, _ := errAttrs(t, err); code != errs.CodeInvalidInput || u.count() != 0 {
		t.Fatalf("code %s after %d calls, want invalid_input before any call", code, u.count())
	}
}

func TestDo_bodyThatFailsToReopenIsATransportFailure(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	req := get(t, "/v1/wallets")
	reopen := errs.New(errs.CodeInternal, "test.GetBody")
	req.GetBody = func() (io.ReadCloser, error) { return nil, reopen }

	err := mustFail(t.Context(), t, client(u), req)
	wantFailure(t, err, errs.CodeUpstreamUnavailable, 0, 1)
	if !errors.Is(err, reopen) || u.count() != 0 {
		t.Fatalf("err = %v after %d calls, want the GetBody error before any call", err, u.count())
	}
}

type failingClose struct{ io.Reader }

func (failingClose) Close() error { return errs.New(errs.CodeInternal, "test.Close") }

func TestResponseBodyClose_wrapsTheUnderlyingError(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingClose{strings.NewReader("")}, Request: r}, nil
	}}}

	resp, err := client(u).Do(t.Context(), get(t, "/v1/users"))
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := errAttrs(t, resp.Body.Close()); code != errs.CodeUpstreamUnavailable {
		t.Fatalf("close code = %s, want upstream_unavailable", code)
	}
}

func TestDo_startsAClientSpanPerAttemptAndPropagatesIt(t *testing.T) {
	t.Parallel()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, parent := tp.Tracer("test").Start(t.Context(), "handler")
	u := &upstream{replies: []reply{status(http.StatusServiceUnavailable), netErr, status(http.StatusOK)}}
	c := client(u, httpclient.WithRetry(3, 0, 0))

	mustOK(ctx, t, c, get(t, "/v1/users"))
	parent.End()

	spans := rec.Ended()
	if len(spans) != 4 {
		t.Fatalf("ended %d spans, want 3 attempts and the parent", len(spans))
	}
	wantStatus := []codes.Code{codes.Error, codes.Error, codes.Unset}
	for i, s := range spans[:3] {
		wantAttemptSpan(t, s, u.reqs[i], parent.SpanContext(), i+1, wantStatus[i])
	}
	if !hasAttr(spans[2].Attributes(), attribute.Int("http.response.status_code", http.StatusOK)) {
		t.Fatalf("final span attrs %v, want status code 200", spans[2].Attributes())
	}
}

func wantAttemptSpan(
	t *testing.T, s sdktrace.ReadOnlySpan, sent *http.Request, parent trace.SpanContext, attempt int, status codes.Code,
) {
	t.Helper()
	if s.SpanKind() != trace.SpanKindClient || s.Name() != "privy GET" || s.Parent().SpanID() != parent.SpanID() {
		t.Fatalf("attempt %d span = %s kind %v parent %v", attempt, s.Name(), s.SpanKind(), s.Parent().SpanID())
	}
	got := trace.SpanContextFromContext(extract(sent))
	if got.SpanID() != s.SpanContext().SpanID() || got.TraceID() != parent.TraceID() {
		t.Fatalf("attempt %d sent traceparent %v, want span %v", attempt, got, s.SpanContext())
	}
	if s.Status().Code != status || !hasAttr(s.Attributes(), attribute.Int("attempt", attempt)) {
		t.Fatalf("attempt %d span status %v attrs %v", attempt, s.Status(), s.Attributes())
	}
	for _, a := range s.Attributes() {
		key := strings.ToLower(string(a.Key))
		if strings.Contains(key, "url") || strings.Contains(key, "header") || strings.Contains(key, "authorization") {
			t.Fatalf("attempt %d span carries %s, want no URL or header attributes", attempt, a.Key)
		}
	}
}

func TestDo_propagatesARemoteParentWithoutARecordingSpan(t *testing.T) {
	t.Parallel()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	remote := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, Remote: true})
	u := &upstream{replies: []reply{status(http.StatusOK)}}

	mustOK(trace.ContextWithRemoteSpanContext(t.Context(), remote), t, client(u), get(t, "/v1/users"))
	if got := u.reqs[0].Header.Get("traceparent"); got != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00" {
		t.Fatalf("traceparent = %q", got)
	}
}

func TestDo_logsEachScheduledRetry(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)))
		u := &upstream{replies: []reply{status(http.StatusTooManyRequests, "Retry-After", "2"), status(http.StatusOK)}}
		c := client(u, httpclient.WithRetry(2, time.Second, time.Second))

		req := get(t, "/v1/users?api-key=secret")
		req.Header.Set("Authorization", "Bearer secret")
		mustOK(ctx, t, c, req)
		var line map[string]any
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatalf("log %q: %v", buf.String(), err)
		}
		if line["level"] != "WARN" || line["msg"] != "httpclient.retry" || line["upstream"] != "privy" ||
			line["attempt"] != 1.0 || line["status"] != 429.0 || line["delay"] != float64(2*time.Second) {
			t.Fatalf("retry line = %v", line)
		}
		keys := slices.Sorted(maps.Keys(line))
		want := []string{"attempt", "delay", "level", "msg", "status", "time", "upstream"}
		if !slices.Equal(keys, want) {
			t.Fatalf("retry line keys = %v, want exactly %v", keys, want)
		}
	})
}

func TestResponseBodyClose_cancelsTheCallContext(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusOK)}}

		mustOK(t.Context(), t, client(u), get(t, "/v1/users"))
		if err := u.reqs[0].Context().Err(); !errors.Is(err, context.Canceled) {
			t.Fatalf("request context after Body.Close = %v, want context.Canceled", err)
		}
	})
}

func TestDo_propagatesThroughTheGlobalPropagator(t *testing.T) {
	t.Parallel()
	member, err := baggage.NewMember("tenant", "cabal1")
	if err != nil {
		t.Fatal(err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatal(err)
	}
	u := &upstream{replies: []reply{status(http.StatusOK)}}

	mustOK(baggage.ContextWithBaggage(t.Context(), bag), t, client(u), get(t, "/v1/users"))
	if got := u.reqs[0].Header.Get("baggage"); got != "tenant=cabal1" {
		t.Fatalf("baggage header = %q, want the global propagator to inject it", got)
	}
}

func tripOnFirstFailure() httpclient.Option {
	return httpclient.WithBreaker(gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
	})
}

func TestDo_localBodyFailureDoesNotTripTheBreaker(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	c := client(u, tripOnFirstFailure())
	req := get(t, "/v1/wallets")
	req.GetBody = func() (io.ReadCloser, error) { return nil, errs.New(errs.CodeInternal, "test.GetBody") }

	if code, _ := errAttrs(t, mustFail(t.Context(), t, c, req)); code != errs.CodeUpstreamUnavailable {
		t.Fatalf("code = %s, want upstream_unavailable", code)
	}
	if got := mustOK(t.Context(), t, c, get(t, "/v1/users")); got.status != http.StatusOK {
		t.Fatalf("status = %d, want the breaker still closed after a local body failure", got.status)
	}
}

func TestDo_callerDeadlineDoesNotTripTheBreakerButTheClientDeadlineDoes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{hang, status(http.StatusOK), hang, status(http.StatusOK)}}
		c := client(u, httpclient.WithTimeout(5*time.Second), tripOnFirstFailure())
		short, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		wantFailure(t, mustFail(short, t, c, get(t, "/v1/users")), errs.CodeUpstreamTimeout, 0, 1)
		if got := mustOK(t.Context(), t, c, get(t, "/v1/users")); got.status != http.StatusOK {
			t.Fatalf("status = %d, want the breaker still closed after the caller's own deadline", got.status)
		}

		wantFailure(t, mustFail(t.Context(), t, c, get(t, "/v1/users")), errs.CodeUpstreamTimeout, 0, 1)
		if err := mustFail(t.Context(), t, c, get(t, "/v1/users")); !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("err = %v, want the client's own deadline to trip the breaker", err)
		}
	})
}

func TestDo_keepsTheCallersOwnBreakerExclusions(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{netErr, status(http.StatusOK)}}
	c := client(u, httpclient.WithBreaker(gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
		IsExcluded:  func(err error) bool { return errs.CodeOf(err) == errs.CodeUpstreamUnavailable },
	}))

	wantFailure(t, mustFail(t.Context(), t, c, get(t, "/v1/users")), errs.CodeUpstreamUnavailable, 0, 1)
	if got := mustOK(t.Context(), t, c, get(t, "/v1/users")); got.status != http.StatusOK {
		t.Fatalf("status = %d, want the caller's exclusion to keep the breaker closed", got.status)
	}
}

func TestNew_panicsOnMissingTimeoutOrRelativeBaseURL(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string][]httpclient.Option{
		"no timeout":   nil,
		"relative url": {httpclient.WithTimeout(time.Second), httpclient.WithBaseURL("/privy")},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: New did not panic", name)
				}
			}()
			httpclient.New("privy", opts...)
		}()
	}
}

func hasAttr(attrs []attribute.KeyValue, want attribute.KeyValue) bool {
	for _, a := range attrs {
		if a == want {
			return true
		}
	}
	return false
}

func extract(r *http.Request) context.Context {
	return propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
}

func now() time.Time { return clock.Real{}.Now() }

func TestDo_aRetryAfterOfZeroSecondsRetriesAtOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{
			replies: []reply{status(http.StatusServiceUnavailable, "Retry-After", "0"), status(http.StatusOK)},
		}
		c := client(u, httpclient.WithRetry(2, time.Hour, time.Hour), httpclient.WithFullDelay())

		got := mustOK(t.Context(), t, c, get(t, "/v1/users"))

		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want 200 on the retry", got.status)
		}
		equalGaps(t, u.gaps(), 0)
	})
}

func TestDo_aDelayAtTheCeilingStaysThereWithoutOverflowingOnTheNextRetry(t *testing.T) {
	t.Parallel()
	const ceiling = time.Duration(1 << 62)
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{
			status(http.StatusServiceUnavailable, "Retry-After", "0"),
			status(http.StatusServiceUnavailable),
			status(http.StatusOK),
		}}
		c := client(
			u,
			httpclient.WithRetry(3, ceiling, ceiling),
			httpclient.WithTimeout(time.Duration(math.MaxInt64)),
			httpclient.WithFullDelay(),
		)

		got := mustOK(t.Context(), t, c, get(t, "/v1/users"))

		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want 200 on the third attempt", got.status)
		}
		equalGaps(t, u.gaps(), 0, ceiling)
	})
}

type headerWaitTimeoutError struct{}

func (headerWaitTimeoutError) Error() string { return "net/http: timeout awaiting response headers" }

func (headerWaitTimeoutError) Timeout() bool { return true }

func TestDo_aResponseHeaderTimeoutIsAnUpstreamTimeout(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{func(*http.Request) (*http.Response, error) {
		return nil, headerWaitTimeoutError{}
	}}}

	err := mustFail(t.Context(), t, client(u), get(t, "/v1/users"))

	wantFailure(t, err, errs.CodeUpstreamTimeout, 0, 1)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("err = %v, want the transport's response-header timeout", err)
	}
}

func TestNew_givesEachClientItsOwnTransport(t *testing.T) {
	t.Parallel()
	a := httpclient.New("a", httpclient.WithTimeout(time.Minute))
	b := httpclient.New("b", httpclient.WithTimeout(2*time.Minute))
	t.Cleanup(a.CloseIdleConnections)
	t.Cleanup(b.CloseIdleConnections)

	assertOwnTransport(t, httpclient.RoundTripper(a), time.Minute)
	assertOwnTransport(t, httpclient.RoundTripper(b), 2*time.Minute)
	if httpclient.RoundTripper(a) == httpclient.RoundTripper(b) {
		t.Fatal("two clients share a transport")
	}
}

func assertOwnTransport(t *testing.T, rt http.RoundTripper, want time.Duration) {
	t.Helper()
	if rt == nil || rt == http.DefaultTransport {
		t.Fatalf("transport = %p, want a clone of DefaultTransport", rt)
	}
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", rt)
	}
	if tr.ResponseHeaderTimeout != want {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", tr.ResponseHeaderTimeout, want)
	}
}

type heldConn struct {
	url     string
	entered <-chan struct{}
	release func()
}

func newHeldConn(t *testing.T) heldConn {
	t.Helper()
	releaseCh := make(chan struct{})
	entered := make(chan struct{}, 1)
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		holdOrWarm(w, r, entered, releaseCh)
	}))
	t.Cleanup(srv.Close)
	return heldConn{
		url:     srv.URL,
		entered: entered,
		release: func() { once.Do(func() { close(releaseCh) }) },
	}
}

func holdOrWarm(w http.ResponseWriter, r *http.Request, entered chan<- struct{}, release <-chan struct{}) {
	if r.URL.Path == "/warm" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	select {
	case <-r.Context().Done():
		return
	case entered <- struct{}{}:
	}
	select {
	case <-r.Context().Done():
		return
	case <-release:
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "done")
}

func quietServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func readHeldBody(ctx context.Context, c *httpclient.Client, req *http.Request) error {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if string(body) != "done" {
		return errs.New(errs.CodeInternal, "test.hold", slog.String("body", string(body)))
	}
	return nil
}

func waitHeld(t *testing.T, entered <-chan struct{}, done <-chan error) {
	t.Helper()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("in-flight request ended before the handler held it: %v", err)
	}
}

func waitFinished(t *testing.T, done <-chan error) {
	t.Helper()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCloseIdleConnections_leavesTheOtherClientsInFlightRequest(t *testing.T) {
	t.Parallel()
	hold := newHeldConn(t)
	other := quietServer(t)
	clientA := httpclient.New("hold", httpclient.WithBaseURL(hold.url), httpclient.WithTimeout(time.Minute))
	clientB := httpclient.New("other", httpclient.WithBaseURL(other), httpclient.WithTimeout(time.Minute))
	t.Cleanup(clientA.CloseIdleConnections)
	t.Cleanup(clientB.CloseIdleConnections)

	mustOK(t.Context(), t, clientA, get(t, "/warm"))
	mustOK(t.Context(), t, clientB, get(t, "/idle"))

	done := make(chan error, 1)
	finished := make(chan struct{})
	req := get(t, "/hold")
	go func() {
		defer close(finished)
		done <- readHeldBody(t.Context(), clientA, req)
	}()
	t.Cleanup(func() { <-finished })
	t.Cleanup(hold.release)

	waitHeld(t, hold.entered, done)
	clientB.CloseIdleConnections()
	hold.release()
	waitFinished(t, done)
}
