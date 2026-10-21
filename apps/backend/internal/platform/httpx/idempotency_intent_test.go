package httpx

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type lossyStore struct {
	*db.IdempotencyStore
	failComplete atomic.Bool
}

func (s *lossyStore) Complete(ctx context.Context, ref db.ClaimRef, resp db.StoredResponse) error {
	if s.failComplete.Load() {
		return dbDown()
	}
	return s.IdempotencyStore.Complete(ctx, ref, resp)
}

type moneyProbe struct {
	uow     *db.UnitOfWork
	clock   *testkit.Clock
	effects atomic.Int32
	fail    atomic.Bool
}

func (p *moneyProbe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var id uuid.UUID
	replayed := false
	err := p.uow.Do(r.Context(), func(ctx context.Context, tx db.Tx) error {
		var err error
		id, replayed, err = IntentKey(ctx, "probe.send").Bind(ctx, tx.Queries(), uuid.New(), p.clock.Now())
		return err
	})
	if err != nil {
		Problem(w, r, err)
		return
	}
	if !replayed {
		p.effects.Add(1)
	}
	if p.fail.Load() {
		Problem(w, r, errs.New(errs.CodeInternal, "probe.send"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"id":"` + id.String() + `"}`))
}

func newMoneyProbe(t *testing.T) (*moneyProbe, *lossyStore) {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	pool := testkit.DB(t)
	return &moneyProbe{uow: db.New(pool, testkit.NewIDs(5), clk), clock: clk},
		&lossyStore{IdempotencyStore: db.NewIdempotencyStore(pool, clk)}
}

func TestIdempotency_aLostResponseSaveRetriedAfterTheTakeoverRepeatsNoSideEffect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	probe, store := newMoneyProbe(t)
	store.failComplete.Store(true)
	handler := idempotent(h, store, probe)

	first := send(t, handler, http.MethodPost, "/v1/send", "k1", `{"amount":1}`)
	if first.Code != http.StatusAccepted || probe.effects.Load() != 1 {
		t.Fatalf("first = %d with %d effects, want 202 and 1", first.Code, probe.effects.Load())
	}
	probe.clock.Advance(5*time.Minute + time.Second)
	store.failComplete.Store(false)

	again := send(t, handler, http.MethodPost, "/v1/send", "k1", `{"amount":1}`)
	if again.Code != http.StatusAccepted || again.Body.String() != first.Body.String() {
		t.Fatalf("retry = %d %q, want the first answer %q", again.Code, again.Body, first.Body)
	}
	if probe.effects.Load() != 1 {
		t.Fatalf("side effects = %d after the retry, want exactly 1", probe.effects.Load())
	}
}

func TestIdempotency_aServerErrorAfterACommittedEffectResumesTheIntentOnRetry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	probe, store := newMoneyProbe(t)
	handler := idempotent(h, store, probe)

	probe.fail.Store(true)
	rec := send(t, handler, http.MethodPost, "/v1/send", "k1", `{"amount":1}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("first = %d, want 500", rec.Code)
	}
	probe.fail.Store(false)
	rec = send(t, handler, http.MethodPost, "/v1/send", "k1", `{"amount":1}`)
	if rec.Code != http.StatusAccepted || probe.effects.Load() != 1 {
		t.Fatalf("retry = %d with %d effects, want 202 and still 1", rec.Code, probe.effects.Load())
	}
}

func TestIdempotency_aRequestWithNoKeyScopeHasNoIntentKey(t *testing.T) {
	t.Parallel()
	if k := IntentKey(t.Context(), "x"); k.Bound() {
		t.Fatalf("IntentKey outside the middleware = %+v, want unbound", k)
	}
}
