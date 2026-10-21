package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
)

func TestAuthorizationRefusal_namesWhyTheConfirmationWasRefused(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	cmd := Withdraw{SessionID: "session-phone"}
	row := func(edit func(*sqlc.GetWithdrawalAuthorizationRow)) sqlc.GetWithdrawalAuthorizationRow {
		r := sqlc.GetWithdrawalAuthorizationRow{SessionID: "session-phone", ExpiresAt: now.Add(time.Minute)}
		edit(&r)
		return r
	}
	lookupFailed := errs.New(errs.CodeDBUnavailable, "test.lookup")
	for name, tc := range map[string]struct {
		row    sqlc.GetWithdrawalAuthorizationRow
		lookup error
		want   errs.Code
	}{
		"unknown":       {lookup: sql.ErrNoRows, want: errs.CodeStepUpRequired},
		"used":          {row: row(func(r *sqlc.GetWithdrawalAuthorizationRow) { r.Consumed = true }), want: errs.CodeStepUpRequired},
		"expired":       {row: row(func(r *sqlc.GetWithdrawalAuthorizationRow) { r.ExpiresAt = now }), want: errs.CodeStepUpRequired},
		"other session": {row: row(func(r *sqlc.GetWithdrawalAuthorizationRow) { r.SessionID = "tablet" }), want: errs.CodeStepUpRequired},
		"changed":       {row: row(func(*sqlc.GetWithdrawalAuthorizationRow) {}), want: errs.CodeStepUpMismatch},
		"lookup failed": {lookup: lookupFailed, want: errs.CodeDBUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := errs.CodeOf(authorizationRefusal(tc.row, tc.lookup, cmd, now)); got != tc.want {
				t.Fatalf("authorizationRefusal = %s, want %s", got, tc.want)
			}
		})
	}
}
