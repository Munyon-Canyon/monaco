package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stubStore struct {
	begin    func(ctx context.Context, actor, key string, hash []byte) (db.Claim, error)
	complete func(ctx context.Context, actor, key string, resp db.StoredResponse) error
	release  func(ctx context.Context, actor, key string) error
}

func (s stubStore) Begin(ctx context.Context, actor, key string, hash []byte) (db.Claim, error) {
	if s.begin == nil {
		return db.Claim{}, errs.New(errs.CodeInternal, "stubStore.Begin")
	}
	return s.begin(ctx, actor, key, hash)
}

func (s stubStore) Complete(ctx context.Context, actor, key string, resp db.StoredResponse) error {
	if s.complete == nil {
		return errs.New(errs.CodeInternal, "stubStore.Complete")
	}
	return s.complete(ctx, actor, key, resp)
}

func (s stubStore) Release(ctx context.Context, actor, key string) error {
	if s.release == nil {
		return errs.New(errs.CodeInternal, "stubStore.Release")
	}
	return s.release(ctx, actor, key)
}

func realStore(t *testing.T, h *harness) *db.IdempotencyStore {
	t.Helper()
	return db.NewIdempotencyStore(testkit.DB(t), h.deps.Clock)
}

func asActor(a auth.Actor, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithActor(r.Context(), a)))
	})
}

func idempotent(h *harness, store IdempotencyStore, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", Idempotency(store)(next))
	return h.deps.wrap(asActor(auth.Actor{Kind: auth.ActorUser, ID: "u1"}, mux))
}

type countingHandler struct {
	calls atomic.Int32
	serve func(w http.ResponseWriter, r *http.Request)
}

func (c *countingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.calls.Add(1)
	c.serve(w, r)
}

func createsThing(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Add("X-Thing", "1")
	w.Header().Add("X-Thing", "2")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"echo":` + string(body) + `}`))
}

func send(t *testing.T, handler http.Handler, method, target, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func sameResponse(t *testing.T, first, second *httptest.ResponseRecorder) {
	t.Helper()
	if first.Code != second.Code || first.Body.String() != second.Body.String() ||
		len(first.Header()) != len(second.Header()) {
		t.Fatalf("replay differs:\n%d %v %q\n%d %v %q", first.Code, first.Header(), first.Body,
			second.Code, second.Header(), second.Body)
	}
	for k, v := range first.Header() {
		if got := second.Header().Values(k); strings.Join(got, "|") != strings.Join(v, "|") {
			t.Fatalf("replay header %s = %v, want %v", k, got, v)
		}
	}
}

const optOutSpec = `openapi: 3.1.0
info: {title: t, version: "1"}
paths:
  /v1/opted-out:
    post:
      operationId: postOptedOut
      x-idempotent: false
      responses: {"201": {description: created}}
  /v1/opted-in:
    post:
      operationId: postOptedIn
      x-idempotent: true
      responses: {"201": {description: created}}
  /v1/plain:
    post:
      operationId: postPlain
      responses: {"201": {description: created}}
`

func TestIdempotency_onlyAnOperationDeclaringXIdempotentFalseSkipsTheKey(t *testing.T) {
	t.Parallel()
	c, err := loadContract([]byte(optOutSpec))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		path   string
		status int
		calls  int32
	}{
		"false":       {"/v1/opted-out", http.StatusCreated, 1},
		"true":        {"/v1/opted-in", http.StatusBadRequest, 0},
		"no property": {"/v1/plain", http.StatusBadRequest, 0},
	} {
		h := newHarness(t)
		next := &countingHandler{serve: createsThing}
		rec := send(t, h.deps.wrap(c.resolve(Idempotency(stubStore{})(next))), http.MethodPost, tc.path, "", `{}`)
		if rec.Code != tc.status || next.calls.Load() != tc.calls {
			t.Errorf("%s: POST %s without a key = %d after %d handler calls, want %d and %d",
				name, tc.path, rec.Code, next.calls.Load(), tc.status, tc.calls)
		}
	}
}

func TestIdempotency_requiresTheHeaderOnMutatingMethodsOnly(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		method, key string
		status      int
		calls       int32
	}{
		"post without key":    {http.MethodPost, "", 400, 0},
		"put without key":     {http.MethodPut, "", 400, 0},
		"patch without key":   {http.MethodPatch, "", 400, 0},
		"delete without key":  {http.MethodDelete, "", 400, 0},
		"post with long key":  {http.MethodPost, strings.Repeat("k", 256), 400, 0},
		"get without key":     {http.MethodGet, "", 201, 1},
		"head without key":    {http.MethodHead, "", 201, 1},
		"options without key": {http.MethodOptions, "", 201, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			next := &countingHandler{serve: createsThing}
			rec := send(t, idempotent(h, stubStore{}, next), tc.method, "/v1/things", tc.key, `{}`)
			if rec.Code != tc.status || next.calls.Load() != tc.calls {
				t.Fatalf("%s = %d after %d handler calls, want %d and %d", name, rec.Code, next.calls.Load(),
					tc.status, tc.calls)
			}
			if tc.status == http.StatusBadRequest {
				expectMissingKeyProblem(t, h, rec)
			}
		})
	}
}

func TestIdempotency_acceptsAKeyAtTheLengthLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	rec := send(t, idempotent(h, realStore(t, h), next), http.MethodPost, "/v1/things", strings.Repeat("k", 255), `{}`)
	if rec.Code != http.StatusCreated || next.calls.Load() != 1 {
		t.Fatalf("255-byte key = %d after %d handler calls, want 201 and 1", rec.Code, next.calls.Load())
	}
}

func expectMissingKeyProblem(t *testing.T, h *harness, rec *httptest.ResponseRecorder) {
	t.Helper()
	if p := decodeProblem(t, rec); p.Code != "invalid_input" {
		t.Fatalf("problem = %+v, want invalid_input", p)
	}
	line := linesNamed(h.logs.lines(t), "http.problem")[0]
	if detail, _ := line["detail"].(map[string]any); detail["header"] != IdempotencyKeyHeader {
		t.Fatalf("problem line = %v, want the header name in detail", line)
	}
}

func TestIdempotency_replaysACompletedResponseByteForByte(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	handler := idempotent(h, realStore(t, h), next)

	first := send(t, handler, http.MethodPost, "/v1/things?x=1", "k1", `{"amount":5}`)
	second := send(t, handler, http.MethodPost, "/v1/things?x=1", "k1", `{"amount":5}`)
	if first.Code != http.StatusCreated || first.Body.String() != `{"echo":{"amount":5}}` ||
		strings.Join(first.Header().Values("X-Thing"), ",") != "1,2" {
		t.Fatalf("first = %d %v %q", first.Code, first.Header(), first.Body)
	}
	sameResponse(t, first, second)
	if next.calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", next.calls.Load())
	}
	replayed := linesNamed(h.logs.lines(t), "http.idempotency.replayed")
	if len(replayed) != 1 || replayed[0]["idempotency_key"] != "k1" || replayed[0]["status"] != 201.0 ||
		replayed[0]["actor"] != "user:u1" {
		t.Fatalf("replayed lines = %v, want one with the visible key, status and actor", replayed)
	}
}

func TestIdempotency_aHandlerThatWritesNothingReplaysAs200(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: func(http.ResponseWriter, *http.Request) {}}
	handler := idempotent(h, realStore(t, h), next)
	first := send(t, handler, http.MethodPost, "/v1/things", "k1", ``)
	second := send(t, handler, http.MethodPost, "/v1/things", "k1", ``)
	if first.Code != http.StatusOK || first.Body.Len() != 0 || next.calls.Load() != 1 {
		t.Fatalf("first = %d %q after %d calls", first.Code, first.Body, next.calls.Load())
	}
	sameResponse(t, first, second)
}

func TestIdempotency_aDifferentRequestUnderTheSameKeyIsAMismatch(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ target, body string }{
		"other body":  {"/v1/things", `{"amount":6}`},
		"other query": {"/v1/things?x=2", `{"amount":5}`},
		"other path":  {"/v1/others", `{"amount":5}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			next := &countingHandler{serve: createsThing}
			handler := idempotent(h, realStore(t, h), next)
			if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{"amount":5}`); rec.Code != 201 {
				t.Fatalf("first = %d", rec.Code)
			}
			rec := send(t, handler, http.MethodPost, tc.target, "k1", tc.body)
			if p := decodeProblem(t, rec); rec.Code != http.StatusConflict || p.Code != "idempotency_mismatch" ||
				p.Retryable {
				t.Fatalf("second = %d %+v, want 409 idempotency_mismatch", rec.Code, p)
			}
			if next.calls.Load() != 1 {
				t.Fatalf("handler ran %d times, want 1", next.calls.Load())
			}
			line := linesNamed(h.logs.lines(t), "http.problem")[0]
			if detail, _ := line["detail"].(map[string]any); detail["idempotency_key"] != "k1" {
				t.Fatalf("problem line = %v, want idempotency_key visible", line)
			}
		})
	}
}

func TestIdempotency_keysAreScopedPerActor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store := realStore(t, h)
	next := &countingHandler{serve: createsThing}
	mux := http.NewServeMux()
	mux.Handle("/", Idempotency(store)(next))
	for _, a := range []auth.Actor{
		{Kind: auth.ActorUser, ID: "u1"}, {Kind: auth.ActorUser, ID: "u2"}, {Kind: auth.ActorAgent, ID: "u1"},
	} {
		rec := send(t, h.deps.wrap(asActor(a, mux)), http.MethodPost, "/v1/things", "k1", `{}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s = %d, want 201: another actor's key is not this actor's", a.Key(), rec.Code)
		}
	}
	anonymous := send(t, h.deps.wrap(mux), http.MethodPost, "/v1/things", "k1", `{}`)
	if anonymous.Code != http.StatusCreated || next.calls.Load() != 4 {
		t.Fatalf("anonymous = %d after %d calls, want 201 and 4", anonymous.Code, next.calls.Load())
	}
}

func TestIdempotency_aDuplicateWhileInFlightIs409AndTheHandlerRunsOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	next := &countingHandler{serve: func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		createsThing(w, r)
	}}
	handler := idempotent(h, realStore(t, h), next)
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`) }()
	<-entered

	dup := send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`)
	if p := decodeProblem(t, dup); dup.Code != http.StatusConflict || p.Code != "idempotency_in_flight" {
		t.Fatalf("duplicate = %d %+v, want 409 idempotency_in_flight", dup.Code, p)
	}
	close(release)
	rec := <-first
	if rec.Code != http.StatusCreated || next.calls.Load() != 1 {
		t.Fatalf("first = %d after %d calls, want 201 and 1", rec.Code, next.calls.Load())
	}
	sameResponse(t, rec, send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`))
}

func TestIdempotency_racingDuplicatesExecuteTheHandlerExactlyOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	handler := idempotent(h, realStore(t, h), next)
	const n = 6
	results := make(chan *httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() { results <- send(t, handler, http.MethodPost, "/v1/things", "k1", `{"amount":5}`) })
	}
	wg.Wait()
	close(results)
	created, inFlight := 0, 0
	for rec := range results {
		switch rec.Code {
		case http.StatusCreated:
			created++
			if rec.Body.String() != `{"echo":{"amount":5}}` {
				t.Errorf("201 body = %q", rec.Body)
			}
		case http.StatusConflict:
			inFlight++
			if p := decodeProblem(t, rec); p.Code != "idempotency_in_flight" {
				t.Errorf("409 = %+v, want idempotency_in_flight", p)
			}
		default:
			t.Errorf("status %d, want 201 or 409", rec.Code)
		}
	}
	if next.calls.Load() != 1 || created < 1 || created+inFlight != n {
		t.Fatalf("handler ran %d times; %d created, %d in flight; want 1 run and %d responses",
			next.calls.Load(), created, inFlight, n)
	}
}

type failingOnce struct {
	serve  func(w http.ResponseWriter, r *http.Request)
	status int
	msg    string
}

func TestIdempotency_a5xxOrPanicReleasesTheKeyForARetry(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]failingOnce{
		"503": {
			serve: func(w http.ResponseWriter, r *http.Request) {
				Problem(w, r, errs.New(errs.CodeDBUnavailable, "x.Y"))
			},
			status: 503, msg: "http.idempotency.released",
		},
		"500": {
			serve: func(w http.ResponseWriter, r *http.Request) {
				Problem(w, r, errs.New(errs.CodeInternal, "x.Y"))
			},
			status: 500, msg: "http.idempotency.released",
		},
		"panic": {
			serve:  func(http.ResponseWriter, *http.Request) { panic("boom") },
			status: 500, msg: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func (tc failingOnce) run(t *testing.T) {
	t.Helper()
	h := newHarness(t)
	var failing atomic.Bool
	failing.Store(true)
	next := &countingHandler{serve: func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			tc.serve(w, r)
			return
		}
		createsThing(w, r)
	}}
	handler := idempotent(h, realStore(t, h), next)
	if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`); rec.Code != tc.status {
		t.Fatalf("failing request = %d, want %d", rec.Code, tc.status)
	}
	failing.Store(false)
	if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("retry = %d, want 201: the key was released", rec.Code)
	}
	if next.calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2", next.calls.Load())
	}
	if tc.msg == "" {
		return
	}
	released := linesNamed(h.logs.lines(t), tc.msg)
	if len(released) != 1 || released[0]["idempotency_key"] != "k1" || released[0]["status"] != float64(tc.status) {
		t.Fatalf("released lines = %v", released)
	}
}

func TestIdempotency_anExpiredKeyRunsTheHandlerAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	next := &countingHandler{serve: createsThing}
	handler := idempotent(h, db.NewIdempotencyStore(pool, clk), next)
	send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`)
	clk.Advance(25 * time.Hour)
	if _, err := sqlc.New(pool).DeleteIdempotencyKeysBefore(t.Context(), clk.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{"changed":true}`); rec.Code != 201 ||
		next.calls.Load() != 2 {
		t.Fatalf("after expiry = %d with %d calls, want 201 and 2", rec.Code, next.calls.Load())
	}
}

func TestIdempotency_anOversizedBodyIsInvalidInput(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.deps.MaxBodyBytes = 4
	next := &countingHandler{serve: createsThing}
	rec := send(t, idempotent(h, stubStore{}, next), http.MethodPost, "/v1/things", "k1", `0123456789`)
	if p := decodeProblem(t, rec); rec.Code != http.StatusBadRequest || p.Code != "invalid_input" ||
		next.calls.Load() != 0 {
		t.Fatalf("got %d %+v after %d calls, want 400 invalid_input before the handler", rec.Code, p, next.calls.Load())
	}
}

func dbDown() error { return errs.New(errs.CodeDBUnavailable, "db.IdempotencyStore.Begin") }

func owned(context.Context, string, string, []byte) (db.Claim, error) {
	return db.Claim{Outcome: db.ClaimOwned}, nil
}

func TestIdempotency_storeFailure_beginFailsBeforeTheHandler(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	store := stubStore{begin: func(context.Context, string, string, []byte) (db.Claim, error) {
		return db.Claim{}, dbDown()
	}}
	rec := send(t, idempotent(h, store, next), http.MethodPost, "/v1/things", "k1", `{}`)
	if p := decodeProblem(t, rec); rec.Code != 503 || p.Code != "db_unavailable" || next.calls.Load() != 0 {
		t.Fatalf("got %d %+v after %d calls, want 503 db_unavailable and no handler run", rec.Code, p,
			next.calls.Load())
	}
}

func TestIdempotency_storeFailure_unknownOutcomeIsInternal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store := stubStore{begin: func(context.Context, string, string, []byte) (db.Claim, error) {
		return db.Claim{Outcome: 99}, nil
	}}
	rec := send(t, idempotent(h, store, &countingHandler{serve: createsThing}), http.MethodPost, "/v1/t", "k1", ``)
	if p := decodeProblem(t, rec); rec.Code != 500 || p.Code != "internal" {
		t.Fatalf("got %d %+v, want 500 internal", rec.Code, p)
	}
}

func TestIdempotency_storeFailure_completeFailsAfterTheHandlerRan(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	store := stubStore{begin: owned, complete: func(context.Context, string, string, db.StoredResponse) error {
		return dbDown()
	}}
	rec := send(t, idempotent(h, store, next), http.MethodPost, "/v1/things", "k1", `{}`)
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"echo":{}}` {
		t.Fatalf("got %d %q, want the handler's 201 delivered anyway", rec.Code, rec.Body)
	}
	failed := linesNamed(h.logs.lines(t), "http.idempotency.store_failed")
	if len(failed) != 1 || failed[0]["level"] != "ERROR" || failed[0]["idempotency_key"] != "k1" ||
		failed[0]["status"] != 201.0 || failed[0]["err"] != dbDown().Error() {
		t.Fatalf("store_failed lines = %v", failed)
	}
}

func TestIdempotency_storeFailure_releaseFailsAfterA5xx(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: func(w http.ResponseWriter, r *http.Request) {
		Problem(w, r, errs.New(errs.CodeInternal, "x.Y"))
	}}
	store := stubStore{begin: owned, release: func(context.Context, string, string) error { return dbDown() }}
	rec := send(t, idempotent(h, store, next), http.MethodPost, "/v1/things", "k1", `{}`)
	failed := linesNamed(h.logs.lines(t), "http.idempotency.store_failed")
	if rec.Code != 500 || len(failed) != 1 || failed[0]["status"] != 500.0 {
		t.Fatalf("got %d, store_failed lines = %v", rec.Code, failed)
	}
}

func TestHandler_requiresAnIdempotencyStore(t *testing.T) {
	t.Parallel()
	d := newHarness(t).deps
	d.Idempotency = nil
	if h, err := Handler(d, healthOnly{}, openapi.Spec); h != nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handler = %v, %v, want internal and no handler", h, err)
	}
}

func TestIdempotency_anonymousCallersNeverShareAKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := &countingHandler{serve: createsThing}
	mux := http.NewServeMux()
	mux.Handle("/", Idempotency(realStore(t, h))(next))
	handler := h.deps.wrap(mux)
	first := send(t, handler, http.MethodPost, "/v1/session", "k1", `{"token":"alice"}`)
	second := send(t, handler, http.MethodPost, "/v1/session", "k1", `{"token":"bob"}`)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || next.calls.Load() != 2 {
		t.Fatalf("got %d and %d after %d calls, want two 201s: another client's key is not a mismatch", first.Code,
			second.Code, next.calls.Load())
	}
	replay := send(t, handler, http.MethodPost, "/v1/session", "k1", `{"token":"alice"}`)
	sameResponse(t, first, replay)
	if next.calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2: the identical anonymous request replays", next.calls.Load())
	}
	for _, credential := range []string{"Bearer one", "Bearer two"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/session", strings.NewReader(`{}`))
		req.Header.Set(IdempotencyKeyHeader, "k2")
		req.Header.Set("Authorization", credential)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s = %d, want 201", credential, rec.Code)
		}
	}
	if next.calls.Load() != 4 {
		t.Fatalf("handler ran %d times, want 4: the same body under two credentials is two requests", next.calls.Load())
	}
}

func TestIdempotency_anAbandonedInFlightKeyIsTakenOverAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	store := db.NewIdempotencyStore(testkit.DB(t), clk)
	if _, err := store.Begin(t.Context(), "user:u1", "k1", requestHash(
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/things", nil), []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	next := &countingHandler{serve: createsThing}
	handler := idempotent(h, store, next)
	if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("while in flight = %d, want 409", rec.Code)
	}
	clk.Advance(5*time.Minute + time.Second)
	if rec := send(t, handler, http.MethodPost, "/v1/things", "k1", `{}`); rec.Code != http.StatusCreated ||
		next.calls.Load() != 1 {
		t.Fatalf("after the takeover age = %d with %d calls, want 201 and 1", rec.Code, next.calls.Load())
	}
}
