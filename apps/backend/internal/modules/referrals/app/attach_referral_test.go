package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
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
