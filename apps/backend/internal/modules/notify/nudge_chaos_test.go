//go:build faultpoints

package notify_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedChaosNudgeStates(t *testing.T, h testkit.Harness) {
	t.Helper()
	states := []identity.AuthState{
		identity.AuthAwaitingPhone, identity.AuthAwaitingSocials, identity.AuthAwaitingPhone,
		identity.AuthAwaitingPhone,
	}
	for user, state := range states {
		if _, err := h.Pool.Exec(t.Context(), `UPDATE users SET auth_state = $2 WHERE id = $1`,
			chaosRecipient(user), string(state)); err != nil {
			t.Fatalf("set the auth state of user %d: %v", user, err)
		}
	}
}

func chaosNudge(i int) events.Event {
	round, user := i/chaosUsers, i%chaosUsers
	return events.UserNudgeDue{
		V: 1, UserID: chaosRecipient(user), Kind: []string{"add_phone", "link_x"}[(user+round)%2],
		NudgeNumber: round, At: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
	}
}
