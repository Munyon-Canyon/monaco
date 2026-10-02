package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestCanFollow(t *testing.T) {
	t.Parallel()
	me, them := ids.NewUserID(ids.Real{}), ids.NewUserID(ids.Real{})
	tests := []struct {
		name   string
		them   ids.UserID
		status domain.AccountStatus
		want   errs.Code
	}{
		{"active user", them, domain.AccountActive, ""},
		{"suspended user", them, domain.AccountSuspended, ""},
		{"self", me, domain.AccountActive, errs.CodeCannotFollowSelf},
		{"self wins over a banned status", me, domain.AccountBanned, errs.CodeCannotFollowSelf},
		{"unknown user", them, domain.AccountUnknown, errs.CodeUserNotFound},
		{"deleted user", them, domain.AccountDeleted, errs.CodeUserNotFound},
		{"banned user", them, domain.AccountBanned, errs.CodeUserBanned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := domain.CanFollow(me, tt.them, tt.status)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("CanFollow = %v, want nil", err)
				}
				return
			}
			if got := errs.CodeOf(err); err == nil || got != tt.want {
				t.Fatalf("CanFollow = %v (code %s), want %s", err, got, tt.want)
			}
		})
	}
}
