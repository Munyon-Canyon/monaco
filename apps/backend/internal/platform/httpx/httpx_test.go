package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type healthz func(ctx context.Context) (api.GetHealthzResponseObject, error)

func (h healthz) GetHealthz(ctx context.Context, _ api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	return h(ctx)
}

func (healthz) GetStream(context.Context, api.GetStreamRequestObject) (api.GetStreamResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetStream")
}

func (healthz) PostAuthSession(
	context.Context, api.PostAuthSessionRequestObject,
) (api.PostAuthSessionResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.PostAuthSession")
}

func (healthz) GetMe(context.Context, api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetMe")
}

func (healthz) GetHandleAvailability(
	context.Context, api.GetHandleAvailabilityRequestObject,
) (api.GetHandleAvailabilityResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetHandleAvailability")
}

func (healthz) PostDevice(context.Context, api.PostDeviceRequestObject) (api.PostDeviceResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.PostDevice")
}

func (healthz) DeleteDevice(context.Context, api.DeleteDeviceRequestObject) (api.DeleteDeviceResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.DeleteDevice")
}

func (healthz) PostSystemPing(
	context.Context, api.PostSystemPingRequestObject,
) (api.PostSystemPingResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.PostSystemPing")
}

func (healthz) PostProposalVote(
	context.Context, api.PostProposalVoteRequestObject,
) (api.PostProposalVoteResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.PostProposalVote")
}

func (healthz) GetSystemPing(context.Context, api.GetSystemPingRequestObject) (api.GetSystemPingResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetSystemPing")
}

func (healthz) PostCabal(context.Context, api.PostCabalRequestObject) (api.PostCabalResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.PostCabal")
}

func (healthz) GetCabal(context.Context, api.GetCabalRequestObject) (api.GetCabalResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetCabal")
}

func (healthz) GetAssets(context.Context, api.GetAssetsRequestObject) (api.GetAssetsResponseObject, error) {
	return nil, errs.New(errs.CodeNotFound, "test.healthz.GetAssets")
}

type stepClock struct {
	clock.Real
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(5 * time.Millisecond)
	return c.now
}

type fixedIDs struct{ id uuid.UUID }

func (f fixedIDs) NewV7() uuid.UUID { return f.id }

func generatedID() uuid.UUID { return uuid.UUID{0x01, 0x92, 0, 0, 0, 0, 0x70, 0, 0x80} }

type harness struct {
	deps  Deps
	logs  *syncBuffer
	spans *tracetest.SpanRecorder
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	logs := &syncBuffer{}
	spans := tracetest.NewSpanRecorder()
	return &harness{
		deps: Deps{
			Logger:       observability.NewLogger(config.Config{Env: config.EnvProduction}, logs),
			Tracer:       sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)),
			Clock:        &stepClock{},
			IDs:          fixedIDs{generatedID()},
			MaxBodyBytes: 1 << 20,
			Idempotency:  stubStore{},
			Verifier:     stubVerifier(nil),
		},
		logs:  logs,
		spans: spans,
	}
}

func (h *harness) do(
	t *testing.T, handler http.Handler, method, target string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	return serveRaw(t, testkit.HTTP(t, handler), method, target, header)
}

func serveRaw(
	t *testing.T,
	handler http.Handler,
	method, target string,
	header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustHandler(t *testing.T, d Deps, ssi api.StrictServerInterface) http.Handler {
	t.Helper()
	h, err := Handler(d, ssi, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func decodeProblem(t *testing.T, resp *httptest.ResponseRecorder) api.Problem {
	t.Helper()
	return decodeProblemBody(t, resp.Header(), resp.Body)
}

func decodeProblemBody(t *testing.T, header http.Header, body io.Reader) api.Problem {
	t.Helper()
	if ct := header.Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	var p api.Problem
	if err := dec.Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func linesNamed(lines []map[string]any, msg string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["msg"] == msg {
			out = append(out, l)
		}
	}
	return out
}

func TestProblem_notFoundIs404ProblemJSONWithCodeAndTraceID(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := mustHandler(t, h.deps, healthz(func(context.Context) (api.GetHealthzResponseObject, error) {
		return nil, errs.New(errs.CodeNotFound, "cabal.Get", slog.String("cabal_id", "c-secret-attr"))
	}))

	resp := h.do(t, handler, http.MethodGet, "/healthz", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
	p := decodeProblem(t, resp)
	if len(p.TraceId) != 32 || p.TraceId == strings.Repeat("0", 32) {
		t.Fatalf("trace_id = %v, want a 32-hex trace id", p.TraceId)
	}
	want := api.Problem{
		Type: api.AboutBlank, Title: "Not Found", Status: 404, Code: "not_found",
		Message: errs.Message(errs.CodeNotFound), TraceId: p.TraceId, Retryable: false,
	}
	if p != want {
		t.Fatalf("problem = %+v, want %+v", p, want)
	}

	problems := linesNamed(h.logs.lines(t), "http.problem")
	if len(problems) != 1 {
		t.Fatalf("got %d http.problem lines, want 1", len(problems))
	}
	line := problems[0]
	if line["level"] != "INFO" || line["code"] != "not_found" || line["alert"] != false ||
		line["trace_id"] != p.TraceId || line["err"] != "cabal.Get: not_found" {
		t.Fatalf("problem line = %v", line)
	}
	if detail, _ := line["detail"].(map[string]any); detail["cabal_id"] != "c-secret-attr" {
		t.Fatalf("detail = %v, want the error attrs", line["detail"])
	}
}

func TestProblem_bodyNeverCarriesErrOrAttrs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	cause := errs.New(errs.CodeDBUnavailable, "db.Begin", slog.String("dsn_host", "attr-leak"))
	handler := mustHandler(t, h.deps, healthz(func(context.Context) (api.GetHealthzResponseObject, error) {
		return nil, errs.Wrap(cause, errs.CodeUpstreamTimeout, "market.Poll", slog.String("provider", "attr-leak-2"))
	}))
	resp := h.do(t, handler, http.MethodGet, "/healthz", nil)
	body := resp.Body.Bytes()
	for _, leak := range []string{"attr-leak", "db.Begin", "market.Poll", "db_unavailable"} {
		if strings.Contains(string(body), leak) {
			t.Fatalf("body %s leaks %q", body, leak)
		}
	}
	line := linesNamed(h.logs.lines(t), "http.problem")[0]
	detail, _ := line["detail"].(map[string]any)
	if line["level"] != "ERROR" || detail["provider"] != "attr-leak-2" || detail["dsn_host"] != "attr-leak" {
		t.Fatalf("problem line = %v, want ERROR with attrs from every wrapped errs.Error", line)
	}
}

func TestProblem_statusRetryableAndAlertFollowTheCodeTable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err       error
		status    int
		code      api.ErrorCode
		retryable bool
		level     string
		alert     bool
	}{
		{errs.New(errs.CodeUpstreamUnavailable, "jupiter.Quote"), 503, "upstream_unavailable", true, "ERROR", false},
		{errs.New(errs.CodeVersionConflict, "cabal.Rename"), 409, "version_conflict", false, "INFO", false},
		{errs.New(errs.CodeRateLimited, "ratelimit.Middleware"), 429, "rate_limited", true, "INFO", false},
		{io.ErrUnexpectedEOF, 500, "internal", false, "ERROR", true},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			handler := mustHandler(t, h.deps, healthz(func(context.Context) (api.GetHealthzResponseObject, error) {
				return nil, tc.err
			}))
			resp := h.do(t, handler, http.MethodGet, "/healthz", nil)
			p := decodeProblem(t, resp)
			if resp.Code != tc.status || p.Status != tc.status || p.Code != tc.code ||
				p.Retryable != tc.retryable || p.Title != http.StatusText(tc.status) {
				t.Fatalf("got %d %+v", resp.Code, p)
			}
			line := linesNamed(h.logs.lines(t), "http.problem")[0]
			if line["level"] != tc.level || line["alert"] != tc.alert {
				t.Fatalf("problem line = %v, want level %s alert %v", line, tc.level, tc.alert)
			}
		})
	}
}

func getOver(t *testing.T, srv *httptest.Server, path string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func levelLines(lines []map[string]any, level string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["level"] == level {
			out = append(out, l)
		}
	}
	return out
}

func TestPanic_respondsPanicProblemLogsOneErrorAndKeepsServing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls int
	srv := httptest.NewServer(testkit.HTTP(t, mustHandler(t, h.deps, healthz(
		func(context.Context) (api.GetHealthzResponseObject, error) {
			calls++
			if calls == 1 {
				panic("boom")
			}
			return api.GetHealthz200TextResponse("ok\n"), nil
		}))))
	defer srv.Close()

	status, header, body := getOver(t, srv, "/healthz")
	if p := decodeProblemBody(
		t,
		header,
		bytes.NewReader(body),
	); status != http.StatusInternalServerError || p.Code != "panic" ||
		p.Retryable {
		t.Fatalf("first request = %d %+v, want 500 panic", status, p)
	}
	if status, _, body := getOver(t, srv, "/healthz"); status != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("second request = %d %q, want 200 ok", status, body)
	}

	errorLines := levelLines(h.logs.lines(t), "ERROR")
	if len(errorLines) != 1 {
		t.Fatalf("got %d ERROR lines, want 1: %v", len(errorLines), errorLines)
	}
	detail, _ := errorLines[0]["detail"].(map[string]any)
	stack, _ := detail["stack"].(string)
	if errorLines[0]["code"] != "panic" || errorLines[0]["alert"] != true || detail["panic"] != "boom" ||
		!strings.Contains(stack, "serveRecovered") {
		t.Fatalf("panic line = %v", errorLines[0])
	}
}

func TestPanic_afterTheResponseStartedOnlyLogs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := h.deps.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("late")
	}))
	resp := serveRaw(t, handler, http.MethodGet, "/x", nil)
	if resp.Code != http.StatusAccepted || resp.Body.Len() != 0 {
		t.Fatalf("got %d %q, want the 202 already sent and no problem body", resp.Code, resp.Body)
	}
	lines := h.logs.lines(t)
	problem := linesNamed(lines, "http.problem")
	access := linesNamed(lines, "http.request")
	if len(problem) != 1 || problem[0]["code"] != "panic" || len(access) != 1 || access[0]["status"] != 202.0 {
		t.Fatalf("lines = %v", lines)
	}
}

func TestPanic_abortHandlerPropagatesToNetHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := h.deps.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if err, _ := recover().(error); !errors.Is(err, http.ErrAbortHandler) {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", err)
		}
	}()
	serveRaw(t, handler, http.MethodGet, "/x", nil)
}

func TestRequestID_acceptsAWellFormedHeaderAndGeneratesOtherwise(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want string }{
		"accepted":  {"req-1.a:B_2", "req-1.a:B_2"},
		"absent":    {"", generatedID().String()},
		"too long":  {strings.Repeat("a", 129), generatedID().String()},
		"bad chars": {"a b\n", generatedID().String()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			header := http.Header{}
			if tc.in != "" {
				header.Set(RequestIDHeader, tc.in)
			}
			resp := h.do(t, mustHandler(t, h.deps, healthOnly{}), http.MethodGet, "/healthz", header)
			if got := resp.Header().Get(RequestIDHeader); got != tc.want {
				t.Fatalf("response %s = %q, want %q", RequestIDHeader, got, tc.want)
			}
			access := linesNamed(h.logs.lines(t), "http.request")
			if len(access) != 1 || access[0]["request_id"] != tc.want {
				t.Fatalf("access lines = %v, want request_id %q", access, tc.want)
			}
		})
	}
}

func TestAccessLog_andSpanNameTheRouteStatusAndDuration(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if resp := h.do(
		t,
		mustHandler(t, h.deps, healthOnly{}),
		http.MethodGet,
		"/healthz",
		nil,
	); resp.Code != http.StatusOK ||
		resp.Body.String() != "ok\n" ||
		resp.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("GET /healthz = %d %q", resp.Code, resp.Body)
	}
	access := linesNamed(h.logs.lines(t), "http.request")
	if len(access) != 1 {
		t.Fatalf("got %d http.request lines, want 1", len(access))
	}
	want := map[string]any{"level": "INFO", "method": "GET", "route": "/healthz", "status": 200.0, "duration_ms": 5.0}
	for k, v := range want {
		if access[0][k] != v {
			t.Errorf("access line %s = %v, want %v", k, access[0][k], v)
		}
	}
	if access[0]["trace_id"] == nil {
		t.Errorf("access line has no trace_id: %v", access[0])
	}
	spans := h.spans.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /healthz" || spans[0].Status().Code.String() != "Unset" {
		t.Fatalf("spans = %v", spans)
	}
}

func TestSpan_continuesAnIncomingTraceAndMarks5xxAsError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := mustHandler(t, h.deps, healthz(func(context.Context) (api.GetHealthzResponseObject, error) {
		return nil, errs.New(errs.CodeInternal, "x.Y")
	}))
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	header := http.Header{"Traceparent": {"00-" + traceID + "-00f067aa0ba902b7-01"}}
	p := decodeProblem(t, h.do(t, handler, http.MethodGet, "/healthz", header))
	if p.TraceId != traceID {
		t.Fatalf("trace_id = %v, want the incoming %s", p.TraceId, traceID)
	}
	spans := h.spans.Ended()
	if len(spans) != 1 || spans[0].Status().Code.String() != "Error" {
		t.Fatalf("spans = %v, want one span with status Error", spans)
	}
}

func TestUnknownRoute_isANotFoundProblem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, tc := range []struct{ method, target string }{{"GET", "/nope"}, {"POST", "/healthz"}} {
		resp := h.do(t, mustHandler(t, h.deps, healthOnly{}), tc.method, tc.target, nil)
		if p := decodeProblem(t, resp); resp.Code != http.StatusNotFound || p.Code != "not_found" {
			t.Fatalf("%s %s = %d %+v, want 404 not_found", tc.method, tc.target, resp.Code, p)
		}
	}
	for _, line := range linesNamed(h.logs.lines(t), "http.request") {
		if line["route"] != "/" {
			t.Fatalf("access line = %v, want route /", line)
		}
	}
}

func TestInvalidRequest_isAnInvalidInputProblem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil)
	req = req.WithContext(observability.WithLogger(req.Context(), h.deps.Logger))
	rec := httptest.NewRecorder()
	invalidRequest(rec, req, io.ErrUnexpectedEOF)
	resp := rec
	if p := decodeProblem(
		t,
		resp,
	); resp.Code != http.StatusBadRequest || p.Code != "invalid_input" ||
		p.TraceId != strings.Repeat("0", 32) {
		t.Fatalf("got %d %+v, want 400 invalid_input with the zero trace id outside a span", resp.Code, p)
	}
	line := linesNamed(h.logs.lines(t), "http.problem")[0]
	if line["err"] != "httpx.decodeRequest: invalid_input: unexpected EOF" {
		t.Fatalf("problem line = %v", line)
	}
}

func TestMaxBodyBytes_capsTheBodyAndClosesTheConnection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.deps.MaxBodyBytes = 4
	readErrs := make(chan error, 1)
	srv := httptest.NewServer(h.deps.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErrs <- err
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	})))
	defer srv.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/x", strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	var tooLarge *http.MaxBytesError
	if readErr := <-readErrs; !errors.As(readErr, &tooLarge) || tooLarge.Limit != 4 {
		t.Fatalf("read error = %v, want MaxBytesError with limit 4", readErr)
	}
	if !resp.Close {
		t.Fatal("response keeps the connection open after an over-limit body, want Connection: close")
	}
}

type failingWriter struct{ httptest.ResponseRecorder }

func (*failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRecorder_keepsTheFirstStatusAndCodesWriteFailures(t *testing.T) {
	t.Parallel()
	inner := httptest.NewRecorder()
	rec := &recorder{ResponseWriter: inner}
	if rec.statusOr200() != http.StatusOK {
		t.Fatalf("unwritten status = %d, want the 200 net/http sends", rec.statusOr200())
	}
	rec.WriteHeader(http.StatusCreated)
	rec.WriteHeader(http.StatusTeapot)
	if rec.statusOr200() != http.StatusCreated || rec.Unwrap() != inner {
		t.Fatalf("status = %d, want 201 and Unwrap to return the inner writer", rec.statusOr200())
	}
	failing := &recorder{ResponseWriter: &failingWriter{}}
	if _, err := failing.Write([]byte("x")); errs.CodeOf(err) != errs.CodeClientClosed ||
		!errors.Is(err, io.ErrClosedPipe) || failing.statusOr200() != http.StatusOK {
		t.Fatalf("Write = %v, want client_closed wrapping ErrClosedPipe", err)
	}
}

func TestNewServer_takesTimeoutsFromConfig(t *testing.T) {
	t.Parallel()
	h := http.NotFoundHandler()
	srv := NewServer(h, config.Timeouts{HTTPServerRead: 3 * time.Second, HTTPServerWrite: 7 * time.Second})
	if srv.ReadHeaderTimeout != 3*time.Second || srv.ReadTimeout != 3*time.Second ||
		srv.WriteTimeout != 7*time.Second || srv.Handler == nil {
		t.Fatalf("server = %+v", srv)
	}
}
