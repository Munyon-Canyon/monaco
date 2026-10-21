package relayer_test

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	full = relayer.SponsorCostLamports
	fee  = relayer.NetworkFeeLamports
)

type sponsorEnv struct {
	pool *pgxpool.Pool
	uow  *db.UnitOfWork
	now  time.Time
}

func newSponsorEnv(t *testing.T) *sponsorEnv {
	t.Helper()
	pool := testkit.DB(t)
	return &sponsorEnv{
		pool: pool, uow: db.New(pool, testkit.NewIDs(1), clock.Real{}),
		now: clock.Real{}.Now().UTC().Truncate(time.Microsecond),
	}
}

func destination(n int) chain.SolanaAddress {
	return chain.AddressOf(fakes.FixtureKey(fmt.Sprintf("sponsor-destination-%d", n)).Public().(ed25519.PublicKey))
}

func newUser() ids.UserID { return ids.UserIDFrom(uuid.New()) }

func (e *sponsorEnv) request(user ids.UserID, to chain.SolanaAddress, canCreate bool) relayer.SponsorRequest {
	return relayer.SponsorRequest{Ref: uuid.New(), User: user, Destination: to, CanCreate: canCreate, Now: e.now}
}

func (e *sponsorEnv) reserve(
	t *testing.T, b relayer.SponsorBudgets, req relayer.SponsorRequest,
) (relayer.SponsorPlan, error) {
	t.Helper()
	var plan relayer.SponsorPlan
	err := e.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) (err error) {
		plan, err = b.Reserve(ctx, tx.Queries(), req)
		return err
	})
	return plan, err
}

func (e *sponsorEnv) mustReserve(
	t *testing.T, b relayer.SponsorBudgets, req relayer.SponsorRequest,
) relayer.SponsorPlan {
	t.Helper()
	plan, err := e.reserve(t, b, req)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	return plan
}

func (e *sponsorEnv) rows(t *testing.T, ref uuid.UUID) map[string]string {
	t.Helper()
	got := map[string]string{}
	rows, err := e.pool.Query(t.Context(),
		`SELECT scope, lamports::text || ' ' || rent::text || ' ' || (released_at IS NOT NULL)::text
		FROM relayer_sponsor_reservations WHERE ref = $1`, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var scope, value string
		if err := rows.Scan(&scope, &value); err != nil {
			t.Fatal(err)
		}
		got[scope] = value
	}
	return got
}

func TestSponsorReserve_chargesFeeAndRentForAFreshDestination(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	req := e.request(newUser(), destination(0), true)
	plan := e.mustReserve(t, relayer.SponsorBudgets{}, req)
	if plan != (relayer.SponsorPlan{Lamports: full, CreateAccount: true}) {
		t.Fatalf("plan = %+v, want the fee plus the token account rent", plan)
	}
	want := fmt.Sprintf("%d true false", full)
	for scope, got := range e.rows(t, req.Ref) {
		if got != want {
			t.Fatalf("%s row = %q, want %q", scope, got, want)
		}
	}
	if got := len(e.rows(t, req.Ref)); got != 3 {
		t.Fatalf("rows = %d, want one per user, destination and global scope", got)
	}
}

func TestSponsorReserve_chargesOnlyTheFeeWhenTheAccountIsKnownToExist(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	req := e.request(newUser(), destination(0), false)
	plan := e.mustReserve(t, relayer.SponsorBudgets{}, req)
	if plan != (relayer.SponsorPlan{Lamports: fee}) {
		t.Fatalf("plan = %+v, want the network fee alone and no account creation", plan)
	}
}

func TestSponsorReserve_isIdempotentPerRef(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	req := e.request(newUser(), destination(0), true)
	first := e.mustReserve(t, relayer.SponsorBudgets{}, req)
	req.CanCreate = false
	if again := e.mustReserve(t, relayer.SponsorBudgets{}, req); again != first {
		t.Fatalf("second Reserve = %+v, want the first plan %+v", again, first)
	}
	if got := len(e.rows(t, req.Ref)); got != 3 {
		t.Fatalf("rows = %d, want 3: a replay charges nothing more", got)
	}
}

func TestSponsorReserve_routesToTheNoSubsidyPathThenRefuses(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]contention{
		"user":        {relayer.SponsorBudgets{UserDaily: full + fee}, true, false},
		"destination": {relayer.SponsorBudgets{DestinationDaily: full + fee}, false, true},
		"global":      {relayer.SponsorBudgets{GlobalDaily: full + fee}, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSponsorEnv(t)
			next := tc.picker()
			user, to := next(0)
			if plan := e.mustReserve(t, tc.budgets, e.request(user, to, true)); !plan.CreateAccount {
				t.Fatalf("first plan = %+v, want the account created", plan)
			}
			user, to = next(1)
			plan := e.mustReserve(t, tc.budgets, e.request(user, to, true))
			if plan != (relayer.SponsorPlan{Lamports: fee}) {
				t.Fatalf("second plan = %+v, want the no-subsidy path with the fee alone", plan)
			}
			user, to = next(2)
			_, err := e.reserve(t, tc.budgets, e.request(user, to, true))
			wantCode(t, err, errs.CodeRateLimited)
		})
	}
}

type contention struct {
	budgets  relayer.SponsorBudgets
	sameUser bool
	sameTo   bool
}

func (c contention) picker() func(n int) (ids.UserID, chain.SolanaAddress) {
	shared := newUser()
	return func(n int) (ids.UserID, chain.SolanaAddress) {
		user, to := newUser(), destination(n)
		if c.sameUser {
			user = shared
		}
		if c.sameTo {
			to = destination(0)
		}
		return user, to
	}
}

func TestSponsorReserve_refusesWhenTheBudgetIsBelowWhatIsSpent(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	user := newUser()
	e.mustReserve(t, relayer.SponsorBudgets{}, e.request(user, destination(0), true))
	_, err := e.reserve(t, relayer.SponsorBudgets{UserDaily: 1}, e.request(user, destination(1), false))
	wantCode(t, err, errs.CodeRateLimited)
}

func TestSponsorReserve_countsOnlyTheLastDay(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	b, user := relayer.SponsorBudgets{UserDaily: full}, newUser()
	e.mustReserve(t, b, e.request(user, destination(0), true))
	_, err := e.reserve(t, b, e.request(user, destination(1), true))
	wantCode(t, err, errs.CodeRateLimited)
	later := e.request(user, destination(1), true)
	later.Now = e.now.Add(relayer.SponsorWindow + time.Minute)
	if plan := e.mustReserve(t, b, later); !plan.CreateAccount {
		t.Fatalf("plan = %+v, want a fresh day to afford the account again", plan)
	}
}

func TestSponsorRelease_givesTheBudgetBack(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	b, user := relayer.SponsorBudgets{UserDaily: full}, newUser()
	held := e.request(user, destination(0), true)
	e.mustReserve(t, b, held)
	_, err := e.reserve(t, b, e.request(user, destination(1), true))
	wantCode(t, err, errs.CodeRateLimited)
	for range 2 {
		if err := b.Release(t.Context(), e.pool, held.Ref, e.now); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}
	for scope, got := range e.rows(t, held.Ref) {
		if got != fmt.Sprintf("%d true true", full) {
			t.Fatalf("%s row = %q, want it kept and marked released", scope, got)
		}
	}
	if plan := e.mustReserve(t, b, e.request(user, destination(1), true)); !plan.CreateAccount {
		t.Fatalf("plan = %+v, want the released budget spendable again", plan)
	}
	if err := b.Release(t.Context(), e.pool, uuid.New(), e.now); err != nil {
		t.Fatalf("Release of an unknown ref: %v", err)
	}
}

func TestSponsorReserve_concurrentWithdrawalsNeverExceedABudget(t *testing.T) {
	t.Parallel()
	const racers, capacity = 12, 3
	for name, tc := range map[string]contention{
		"user":        {relayer.SponsorBudgets{UserDaily: capacity * full}, true, false},
		"destination": {relayer.SponsorBudgets{DestinationDaily: capacity * full}, false, true},
		"global":      {relayer.SponsorBudgets{GlobalDaily: capacity * full}, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSponsorEnv(t)
			next := tc.picker()
			var wg sync.WaitGroup
			var funded, refused atomic.Int64
			for n := range racers {
				user, to := next(n)
				req := e.request(user, to, true)
				wg.Go(func() {
					plan, err := e.reserve(t, tc.budgets, req)
					switch {
					case err == nil && plan.CreateAccount:
						funded.Add(1)
					case errs.CodeOf(err) == errs.CodeRateLimited:
						refused.Add(1)
					default:
						t.Errorf("Reserve = %+v, %v", plan, err)
					}
				})
			}
			wg.Wait()
			if funded.Load() != capacity || refused.Load() != racers-capacity {
				t.Fatalf("funded %d refused %d, want exactly %d funded of %d",
					funded.Load(), refused.Load(), capacity, racers)
			}
		})
	}
}

type failingExec struct {
	sqlc.DBTX
	failAt int
	calls  int
}

func (f *failingExec) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	f.calls++
	if f.calls == f.failAt {
		return pgconn.CommandTag{}, errs.New(errs.CodeInternal, "test")
	}
	return f.DBTX.Exec(ctx, q, args...)
}

func TestSponsorReserve_failsClosedWhenBudgetStorageFailsAtAnyStep(t *testing.T) {
	t.Parallel()
	e := newSponsorEnv(t)
	const writes = 6
	for step := 1; step <= writes; step++ {
		conn := &failingExec{DBTX: e.pool, failAt: step}
		_, err := relayer.SponsorBudgets{}.Reserve(t.Context(), conn, e.request(newUser(), destination(0), true))
		wantCode(t, err, errs.CodeDBUnavailable)
	}
	conn := &failingExec{DBTX: e.pool, failAt: writes + 1}
	_, err := relayer.SponsorBudgets{}.Reserve(t.Context(), conn, e.request(newUser(), destination(0), true))
	if err != nil {
		t.Fatalf("Reserve with every write working: %v", err)
	}
	if conn.calls != writes {
		t.Fatalf("a funded reservation made %d writes, want %d: update the writes in this test", conn.calls, writes)
	}
	release := relayer.SponsorBudgets{}.Release(t.Context(), &failingExec{DBTX: e.pool, failAt: 1}, uuid.New(), e.now)
	wantCode(t, release, errs.CodeDBUnavailable)
}

func TestSponsorReserve_failsClosedWhenItCannotReadWhatIsSpent(t *testing.T) {
	t.Parallel()
	for column, rename := range map[string]string{"rent": "rent_down", "scope_key": "scope_key_down"} {
		t.Run(column, func(t *testing.T) {
			t.Parallel()
			e := newSponsorEnv(t)
			if _, err := e.pool.Exec(t.Context(),
				`ALTER TABLE relayer_sponsor_reservations RENAME COLUMN `+column+` TO `+rename); err != nil {
				t.Fatal(err)
			}
			_, err := e.reserve(t, relayer.SponsorBudgets{}, e.request(newUser(), destination(0), true))
			wantCode(t, err, errs.CodeDBUnavailable)
		})
	}
}

func TestSponsorReserve_failsClosedOnAnAmountItCannotRead(t *testing.T) {
	t.Parallel()
	const tooBig = "99999999999999999999"
	e := newSponsorEnv(t)
	user, ref := newUser(), uuid.New()
	insert := func(scope, key string, ref uuid.UUID) {
		_, err := e.pool.Exec(t.Context(), `INSERT INTO relayer_sponsor_reservations
			(ref, scope, scope_key, lamports, rent, created_at) VALUES ($1, $2, $3, $4::numeric, true, $5)`,
			ref, scope, key, tooBig, e.now)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("global", "", ref)
	req := e.request(user, destination(0), true)
	req.Ref = ref
	_, err := e.reserve(t, relayer.SponsorBudgets{}, req)
	wantCode(t, err, errs.CodeDecodeFailed)
	insert("user", user.String(), uuid.New())
	_, err = e.reserve(t, relayer.SponsorBudgets{}, e.request(user, destination(0), true))
	wantCode(t, err, errs.CodeDecodeFailed)
}
