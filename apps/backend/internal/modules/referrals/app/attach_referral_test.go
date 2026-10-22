package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestWindowOpen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		card identity.UserCard
		want bool
	}{
		{"six days twenty three hours", identity.UserCard{CreatedAt: now.Add(-(6*24 + 23) * time.Hour)}, true},
		{"seven days", identity.UserCard{CreatedAt: now.Add(-7 * 24 * time.Hour)}, false},
		{"completed recently", identity.UserCard{CreatedAt: now, AuthState: identity.AuthOnboardingCompleted, AuthStateChangedAt: now.Add(-23 * time.Hour)}, true},
		{"completed long ago", identity.UserCard{CreatedAt: now, AuthState: identity.AuthOnboardingCompleted, AuthStateChangedAt: now.Add(-25 * time.Hour)}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := windowOpen(tt.card, now); got != tt.want {
				t.Fatalf("windowOpen = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestReferralInsertError(t *testing.T) {
	t.Parallel()
	if got := referralInsertError(sql.ErrNoRows); errs.CodeOf(got) != errs.CodeReferralAlreadyAttached {
		t.Fatalf("referralInsertError = %v, want referral_already_attached", got)
	}
}

func attributedLines(t *testing.T, source string, attaches int) (errs.Code, int) {
	t.Helper()
	pool := testkit.DB(t)
	clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	generator := testkit.NewIDs(95)
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
	h := NewAttachReferralHandler(AttachReferralDeps{
		UoW: db.New(pool, generator, clock), Resolver: Resolver{Reads: pool, Users: users},
		Users: users, IDs: generator, Clock: clock,
	})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(
		observability.WithActor(t.Context(), "user:"+caller.String()),
		observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	var last error
	for range attaches {
		last = h.Handle(ctx, AttachReferral{CallerID: caller, Code: "k7m4qx2p", Source: source})
	}
	return errs.CodeOf(last), len(logs.Lines(observability.ReferralsAttributed.Name, "source"))
}

func TestAttachReferral_logsTheAttributionOnceAfterCommit(t *testing.T) {
	t.Parallel()
	code, lines := attributedLines(t, "manual", 2)
	if code != errs.CodeReferralAlreadyAttached {
		t.Fatalf("second attach = %s, want referral_already_attached", code)
	}
	if lines != 1 {
		t.Fatalf("referrals.attributed lines = %d, want 1", lines)
	}
}

func TestAttachReferral_doesNotLogARolledBackAttribution(t *testing.T) {
	t.Parallel()
	code, lines := attributedLines(t, "bad", 1)
	if code != errs.CodeInternal {
		t.Fatalf("invalid source = %s, want internal", code)
	}
	if lines != 0 {
		t.Fatalf("referrals.attributed lines = %d on a rolled-back attach, want 0", lines)
	}
}
