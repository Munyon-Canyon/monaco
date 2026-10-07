package referrals_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_referrerAndNewUserShareACabalAndTheNewUserHoldsABalance(t *testing.T) {
	t.Parallel()
	seeded := testkit.Seed(t, testkit.DB(t), "referrer-and-new-user")
	count := map[events.Type]int{}
	for _, s := range seeded {
		count[s.Event.Type()]++
	}
	if len(seeded) != 6 || count[events.TypeUserCreated] != 2 || count[events.TypeCabalCreated] != 1 ||
		count[events.TypeCabalMemberJoined] != 2 || count[events.TypeDepositCredited] != 1 {
		t.Fatalf("seeded %v, want two users, a cabal with two members and one deposit", count)
	}
}
