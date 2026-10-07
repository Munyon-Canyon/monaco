package referrals_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type qualifyFixture struct {
	pool *pgxpool.Pool
	uow  *db.UnitOfWork
	ids  *testkit.IDs
	logs *bytes.Buffer
}

func newQualifyFixture(t *testing.T, seed uint64) qualifyFixture {
	t.Helper()
	pool := testkit.DB(t)
	return qualifyFixture{
		pool: pool, uow: db.New(pool, testkit.NewIDs(seed), testkit.NewClock(deliveredAt())),
		ids: testkit.NewIDs(seed + 1000), logs: &bytes.Buffer{},
	}
}

func (f qualifyFixture) context(t *testing.T) context.Context {
	t.Helper()
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, f.logs))
	return observability.WithActor(ctx, "system:consumer-suite")
}

func (f qualifyFixture) seedReferral(t *testing.T) (referralID, referrer, referee uuid.UUID) {
	t.Helper()
	referralID, referrer, referee = f.ids.NewV7(), f.ids.NewV7(), f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO referrals (id, referrer_id, referee_id, code, code_kind, source, status, created_at)
		VALUES ($1, $2, $3, 'k7m4qx2p', 'random', 'manual', 'attributed', $4)`,
		referralID, referrer, referee, deliveredAt().Add(-time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	return referralID, referrer, referee
}

func (f qualifyFixture) funded(referee uuid.UUID, micros uint64) events.Funded {
	return events.Funded{
		V: 1, TransferID: f.ids.NewV7(), CabalID: f.ids.NewV7(), UserID: referee,
		AmountMicros: money.MicrosFromUint64(micros),
	}
}

func (f qualifyFixture) apply(ctx context.Context, users identity.UserReader, e events.Funded) error {
	return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return adapters.Qualify{Users: users}.Handle(ctx, tx, e, deliveredAt())
	})
}

func (f qualifyFixture) mustApply(t *testing.T, users identity.UserReader, e events.Funded) {
	t.Helper()
	if err := f.apply(f.context(t), users, e); err != nil {
		t.Fatalf("qualify funding of %s micros: %v", e.AmountMicros, err)
	}
}

func (f qualifyFixture) state(t *testing.T, referee uuid.UUID) (string, *time.Time) {
	t.Helper()
	var (
		status string
		at     *time.Time
	)
	if err := f.pool.QueryRow(t.Context(),
		`SELECT status, qualified_at FROM referrals WHERE referee_id = $1`, referee).Scan(&status, &at); err != nil {
		t.Fatal(err)
	}
	return status, at
}

func (f qualifyFixture) qualifiedEvents(t *testing.T) []events.ReferralQualified {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = 'referral.qualified' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.ReferralQualified
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var e events.ReferralQualified
		if err := json.Unmarshal(payload, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f qualifyFixture) wantState(t *testing.T, referee uuid.UUID, status string, qualified bool, emitted int) {
	t.Helper()
	got, at := f.state(t, referee)
	if got != status || (at != nil) != qualified || (qualified && !at.Equal(deliveredAt())) ||
		len(f.qualifiedEvents(t)) != emitted {
		t.Fatalf("status %q qualified_at %v events %d, want %q qualified %v events %d",
			got, at, len(f.qualifiedEvents(t)), status, qualified, emitted)
	}
}

func (f qualifyFixture) wantLog(t *testing.T, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(f.logs.String(), part) {
			t.Fatalf("logs lack %s:\n%s", part, f.logs)
		}
	}
}

func verifiedUsers(user uuid.UUID, verified bool) *fakes.Identity {
	return fakes.NewIdentity([]identity.UserCard{
		{ID: ids.UserIDFrom(user), AccountStatus: identity.AccountActive, PhoneVerified: verified},
	}, nil)
}

type unreachableUsers struct {
	identity.UserReader
	t *testing.T
}

func (u unreachableUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	u.t.Error("identity was called")
	return map[ids.UserID]identity.UserCard{}, nil
}

type racingUsers struct {
	identity.UserReader
	race func()
}

func (u racingUsers) UsersByID(ctx context.Context, in []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	u.race()
	return u.UserReader.UsersByID(ctx, in)
}

func TestReferralQualify_ExactlyTenDollars(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6800)
	referral, referrer, referee := f.seedReferral(t)
	funding := f.funded(referee, 10_000_000)
	f.mustApply(t, verifiedUsers(referee, true), funding)
	f.wantState(t, referee, "qualified", true, 1)
	want := events.ReferralQualified{
		V: 1, ReferralID: referral, Referrer: referrer, Referee: referee, CabalID: funding.CabalID,
		AmountMicros: funding.AmountMicros, QualifiedAt: deliveredAt(),
	}
	if got := f.qualifiedEvents(t)[0]; got != want {
		t.Fatalf("referral.qualified = %+v, want %+v", got, want)
	}
	f.wantLog(t, `"msg":"referrals.qualified"`, `"referral_id":"`+referral.String()+`"`)
}

func TestReferralQualify_BelowMinimum(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6810)
	_, _, referee := f.seedReferral(t)
	f.mustApply(t, unreachableUsers{t: t}, f.funded(referee, 9_999_999))
	f.wantState(t, referee, "attributed", false, 0)
	f.wantLog(t, `"msg":"referrals.qualify_skipped"`, `"reason":"below_minimum"`)
	f.mustApply(t, verifiedUsers(referee, true), f.funded(referee, 10_000_000))
	f.wantState(t, referee, "qualified", true, 1)
}

func TestReferralQualify_PhoneUnverifiedThenVerified(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6820)
	_, _, referee := f.seedReferral(t)
	f.mustApply(t, verifiedUsers(referee, false), f.funded(referee, 10_000_000))
	f.wantState(t, referee, "attributed", false, 0)
	f.wantLog(t, `"msg":"referrals.qualify_skipped"`, `"reason":"phone_unverified"`)
	f.mustApply(t, fakes.NewIdentity(nil, nil), f.funded(referee, 50_000_000))
	f.wantState(t, referee, "attributed", false, 0)
	f.mustApply(t, verifiedUsers(referee, true), f.funded(referee, 10_000_000))
	f.wantState(t, referee, "qualified", true, 1)
}

func TestReferralQualify_NoReferral(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6830)
	stranger := f.ids.NewV7()
	f.mustApply(t, unreachableUsers{t: t}, f.funded(stranger, 10_000_000))
	var rows, emitted int
	if err := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM referrals), (SELECT count(*) FROM events)`).
		Scan(&rows, &emitted); err != nil || rows != 0 || emitted != 0 {
		t.Fatalf("referrals %d events %d err %v, want nothing written", rows, emitted, err)
	}
}

func TestReferralQualify_AlreadyQualifiedOrRejected(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6840)
	_, _, rejected := f.seedReferral(t)
	if _, err := f.pool.Exec(
		t.Context(), `UPDATE referrals SET status = 'rejected', reject_reason = 'self' WHERE referee_id = $1`, rejected,
	); err != nil {
		t.Fatal(err)
	}
	f.mustApply(t, unreachableUsers{t: t}, f.funded(rejected, 10_000_000))
	f.wantState(t, rejected, "rejected", false, 0)
}

func TestReferralQualify_Redelivery(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6850)
	_, _, referee := f.seedReferral(t)
	funding, users := f.funded(referee, 25_000_000), verifiedUsers(referee, true)
	f.mustApply(t, users, funding)
	f.mustApply(t, users, funding)
	f.wantState(t, referee, "qualified", true, 1)
}

func TestReferralQualify_Parallel(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6860)
	_, _, referee := f.seedReferral(t)
	users, start := verifiedUsers(referee, true), make(chan struct{})
	fundings := []events.Funded{f.funded(referee, 10_000_000), f.funded(referee, 12_000_000)}
	failures := make(chan error, len(fundings))
	var wg sync.WaitGroup
	for _, funding := range fundings {
		wg.Go(func() {
			<-start
			failures <- f.apply(f.context(t), users, funding)
		})
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatalf("parallel qualify = %v", err)
		}
	}
	f.wantState(t, referee, "qualified", true, 1)
}

func TestReferralQualify_ParallelDeliveryWinsAfterTheLookup(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 6865)
	_, _, referee := f.seedReferral(t)
	users := racingUsers{UserReader: verifiedUsers(referee, true), race: func() {
		if _, err := f.pool.Exec(t.Context(),
			`UPDATE referrals SET status = 'qualified', qualified_at = $2 WHERE referee_id = $1`,
			referee, deliveredAt().Add(-time.Minute),
		); err != nil {
			t.Error(err)
		}
	}}
	f.mustApply(t, users, f.funded(referee, 10_000_000))
	status, at := f.state(t, referee)
	if status != "qualified" || at == nil || !at.Equal(deliveredAt().Add(-time.Minute)) ||
		len(f.qualifiedEvents(t)) != 0 {
		t.Fatalf("status %q qualified_at %v events %d, want the winner's row and no event",
			status, at, len(f.qualifiedEvents(t)))
	}
}

func TestReferralQualify_AfterAttach(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(deliveredAt().Add(-time.Second))
	generator := testkit.NewIDs(6870)
	referrer, caller := ids.UserIDFrom(generator.NewV7()), ids.UserIDFrom(generator.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`, referrer.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: referrer, AccountStatus: identity.AccountActive},
		{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now(), PhoneVerified: true},
	}, nil)
	uow := db.New(pool, generator, clock)
	attach := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: uow, Resolver: app.Resolver{Reads: pool, Users: users}, Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	if err := attach.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	f := qualifyFixture{pool: pool, uow: uow, ids: generator, logs: &bytes.Buffer{}}
	f.mustApply(t, users, f.funded(caller.UUID(), 10_000_000))
	f.wantState(t, caller.UUID(), "qualified", true, 1)
	if got := f.qualifiedEvents(t)[0]; got.Referrer != referrer.UUID() || got.Referee != caller.UUID() {
		t.Fatalf("referral.qualified = %+v, want referrer %s referee %s", got, referrer, caller)
	}
}

func TestReferralQualify_FailuresKeepTheirCodes(t *testing.T) {
	t.Parallel()
	t.Run("identity unavailable", func(t *testing.T) {
		t.Parallel()
		f := newQualifyFixture(t, 6880)
		_, _, referee := f.seedReferral(t)
		users := verifiedUsers(referee, true)
		users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test.UsersByID"))
		err := f.apply(f.context(t), users, f.funded(referee, 10_000_000))
		if errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Fatalf("qualify = %v, want db_unavailable", err)
		}
	})
	t.Run("event append without an actor rolls the update back", func(t *testing.T) {
		t.Parallel()
		f := newQualifyFixture(t, 6890)
		_, _, referee := f.seedReferral(t)
		err := f.apply(t.Context(), verifiedUsers(referee, true), f.funded(referee, 10_000_000))
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("qualify = %v, want internal", err)
		}
		f.wantState(t, referee, "attributed", false, 0)
	})
	t.Run("update fails", func(t *testing.T) {
		t.Parallel()
		f := newQualifyFixture(t, 6900)
		_, _, referee := f.seedReferral(t)
		for _, stmt := range []string{
			`CREATE FUNCTION refuse_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`,
			`CREATE TRIGGER refuse BEFORE UPDATE ON referrals FOR EACH ROW EXECUTE FUNCTION refuse_update()`,
		} {
			if _, err := f.pool.Exec(t.Context(), stmt); err != nil {
				t.Fatal(err)
			}
		}
		err := f.apply(f.context(t), verifiedUsers(referee, true), f.funded(referee, 10_000_000))
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("qualify = %v, want internal", err)
		}
	})
	t.Run("lookup fails", func(t *testing.T) {
		t.Parallel()
		f := newQualifyFixture(t, 6910)
		_, _, referee := f.seedReferral(t)
		if _, err := f.pool.Exec(t.Context(), `DROP TABLE referrals`); err != nil {
			t.Fatal(err)
		}
		err := f.apply(f.context(t), unreachableUsers{t: t}, f.funded(referee, 10_000_000))
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("qualify = %v, want internal", err)
		}
	})
}
