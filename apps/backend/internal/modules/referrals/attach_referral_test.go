package referrals_test

import (
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestAttachReferral_acceptsCallerCodeAndSource(t *testing.T) {
	t.Parallel()
	cmd := app.AttachReferral{Code: "k7m4qx2p", Source: "manual", IdempotencyKey: "attach-referral-1"}
	if cmd.Code == "" || cmd.Source == "" || cmd.IdempotencyKey == "" {
		t.Fatal("command fields must be retained")
	}
}

func TestAttachReferral_rejectsUnknownCode(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(43)
	id, err := ids.ParseUserID(generator.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	users := fakes.NewIdentity([]identity.UserCard{{ID: id, AccountStatus: identity.AccountActive}}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+id.String())
	err = h.Handle(ctx, app.AttachReferral{CallerID: id, Code: "k7m4qx2p", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeReferralCodeUnknown {
		t.Fatalf("unknown code = %v, want referral_code_unknown", err)
	}
}

func TestAttachReferral_concurrentAttachesLeaveOneReferral(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(42)
	toUser := func() ids.UserID {
		id, err := ids.ParseUserID(generator.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	referrer, caller := toUser(), toUser()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`,
		referrer.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: referrer, AccountStatus: identity.AccountActive},
		{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
	}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	start := make(chan struct{})
	errsSeen := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			ctx := observability.WithActor(t.Context(), "user:"+caller.String())
			errsSeen <- h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "manual"})
		})
	}
	close(start)
	wg.Wait()
	close(errsSeen)
	var ok, attached int
	for err := range errsSeen {
		switch {
		case err == nil:
			ok++
		case errs.CodeOf(err) == errs.CodeReferralAlreadyAttached:
			attached++
		default:
			t.Fatalf("attach = %v", err)
		}
	}
	if ok != 1 || attached != 1 {
		t.Fatalf("successes=%d attached=%d", ok, attached)
	}
}

func TestAttachReferral_attachesARandomCode(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(44)
	toUser := func(raw string) ids.UserID {
		id, err := ids.ParseUserID(raw)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	referrer, caller := toUser(generator.NewV7().String()), toUser(generator.NewV7().String())
	const insertCode = `INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`
	if _, err := pool.Exec(t.Context(), insertCode, referrer.UUID()); err != nil {
		t.Fatal(err)
	}
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: referrer, AccountStatus: identity.AccountActive},
		{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
	}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	if err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeReferralAlreadyAttached {
		t.Fatalf("second attach = %v, want referral_already_attached", err)
	}
}

func TestAttachReferral_rejectsSelfAndClosedWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		card identity.UserCard
		code string
		want errs.Code
	}{
		{"self", identity.UserCard{AccountStatus: identity.AccountActive, CreatedAt: now}, "k7m4qx2p", errs.CodeReferralSelf},
		{"six days twenty three hours", identity.UserCard{AccountStatus: identity.AccountActive, CreatedAt: now.Add(-(6*24 + 23) * time.Hour)}, "k7m4qx2p", ""},
		{"seven days", identity.UserCard{AccountStatus: identity.AccountActive, CreatedAt: now.Add(-7 * 24 * time.Hour)}, "k7m4qx2p", errs.CodeReferralWindowClosed},
		{"completed recently", identity.UserCard{AccountStatus: identity.AccountActive, CreatedAt: now, AuthState: identity.AuthOnboardingCompleted, AuthStateChangedAt: now.Add(-23 * time.Hour)}, "k7m4qx2p", ""},
		{"completed", identity.UserCard{AccountStatus: identity.AccountActive, CreatedAt: now, AuthState: identity.AuthOnboardingCompleted, AuthStateChangedAt: now.Add(-25 * time.Hour)}, "k7m4qx2p", errs.CodeReferralWindowClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attachWithCard(t, now, tc.name == "self", tc.card, tc.code, tc.want)
		})
	}
}

func attachWithCard(t *testing.T, now time.Time, self bool, card identity.UserCard, code string, want errs.Code) {
	t.Helper()
	pool := testkit.DB(t)
	clock := testkit.NewClock(now)
	generator := testkit.NewIDs(91)
	toUser := func() ids.UserID {
		id, err := ids.ParseUserID(generator.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	caller, referrer := toUser(), toUser()
	if self {
		referrer = caller
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`,
		referrer.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	callerCard := card
	callerCard.ID = caller
	referrerCard := identity.UserCard{
		ID:             referrer,
		Handle:         "referrer",
		AccountStatus:  identity.AccountActive,
		FirstDepositAt: ptr(clock.Now()),
	}
	users := fakes.NewIdentity([]identity.UserCard{callerCard, referrerCard}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: code, Source: "manual"})
	if want == "" {
		if err != nil {
			t.Fatalf("attach = %v, want success", err)
		}
		return
	}
	if errs.CodeOf(err) != want {
		t.Fatalf("attach = %v, want %s", err, want)
	}
}

func TestAttachReferral_keepsUserAndDatabaseFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(92)
	toUser := func() ids.UserID {
		id, err := ids.ParseUserID(generator.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	caller, referrer := toUser(), toUser()
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
		{ID: referrer, Handle: "referrer_", AccountStatus: identity.AccountActive, FirstDepositAt: ptr(clock.Now())},
	}, nil)
	users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test.UsersByID"))
	if _, err := users.UserByHandle(t.Context(), "referrer_"); err != nil {
		t.Fatalf("handle lookup = %v", err)
	}
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "referrer_", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("attach = %v, want db_unavailable", err)
	}
}

func TestAttachReferral_rollsBackInvalidSourceAndMissingActor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(93)
	toUser := func() ids.UserID {
		id, err := ids.ParseUserID(generator.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	referrer, caller := toUser(), toUser()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`,
		referrer.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: referrer, AccountStatus: identity.AccountActive},
		{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
	}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "bad"})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("invalid source = %v, want internal", err)
	}
	err = h.Handle(t.Context(), app.AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("missing actor = %v, want internal", err)
	}
}

func TestAttachReferral_rejectsMissingCallerAndBrokenReferralLookup(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(94)
	toUser := func() ids.UserID {
		id, err := ids.ParseUserID(generator.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	caller, referrer := toUser(), toUser()
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: referrer, Handle: "referrer_", AccountStatus: identity.AccountActive, FirstDepositAt: ptr(clock.Now())},
	}, nil)
	h := app.NewAttachReferralHandler(app.AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: app.Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	err := h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "referrer_", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("missing caller = %v", err)
	}
	if _, err := pool.Exec(t.Context(), `DROP TABLE referrals`); err != nil {
		t.Fatal(err)
	}
	err = h.Handle(ctx, app.AttachReferral{CallerID: caller, Code: "referrer_", Source: "manual"})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("broken lookup = %v", err)
	}
}

func ptr(v time.Time) *time.Time { return &v }
