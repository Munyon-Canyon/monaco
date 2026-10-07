package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_referralAttributedHoldsTwoUsersAndTheAttribution(t *testing.T) {
	t.Parallel()
	seeded := testkit.Seed(t, testkit.DB(t), "referral-attributed")
	if len(seeded) != 3 {
		t.Fatalf("seeded %+v, want the referrer, the referee and one attribution", seeded)
	}
	referrer, okA := seeded[0].Event.(events.UserCreated)
	referee, okB := seeded[1].Event.(events.UserCreated)
	attributed, okC := seeded[2].Event.(events.ReferralAttributed)
	if !okA || !okB || !okC {
		t.Fatalf("seeded %+v, want two user.created events and a referral.attributed", seeded)
	}
	if attributed.Referrer != referrer.UserID || attributed.Referee != referee.UserID ||
		seeded[2].Actor != "user:"+referee.UserID.String() {
		t.Fatalf("attribution %+v by %s, want the second user attributed to the first", attributed, seeded[2].Actor)
	}
}
