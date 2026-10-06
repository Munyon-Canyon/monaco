//go:build faultpoints

package social_test

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const referralChaosUsers = 6

func referralChaosUser(i int) ids.UserID {
	return ids.UserIDFrom(uuid.NewSHA1(uuid.Nil, []byte("referral-chaos-user"+strconv.Itoa(i))))
}

func referralChaosPair(i int) (referrer, referee int) {
	for a := range referralChaosUsers {
		for b := a + 1; b < referralChaosUsers; b++ {
			if i == 0 {
				return a, b
			}
			i--
		}
	}
	panic("referralChaosPair: more events than distinct pairs")
}

func referralChaosIdentity() *fakes.Identity {
	cards := make([]identity.UserCard, referralChaosUsers)
	for i := range cards {
		cards[i] = identity.UserCard{
			ID: referralChaosUser(i), Handle: "chaos" + strconv.Itoa(i), AccountStatus: identity.AccountActive,
		}
	}
	cards[referralChaosUsers-1].AccountStatus = identity.AccountBanned
	return fakes.NewIdentity(cards, nil)
}

func TestReferralFollows_Chaos_ConvergesToTheSingleDeliveryRun(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, c := range social.New(deps, social.WithUsers(referralChaosIdentity())).Consumers() {
			if c.Durable == "social_referral_follows" {
				return c
			}
		}
		t.Fatal("social_referral_follows is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		referrer, referee := referralChaosPair(i)
		return events.ReferralAttributed{
			V: 1, ReferralID: uuid.NewSHA1(uuid.Nil, []byte("referral-chaos"+strconv.Itoa(i))),
			Referrer: referralChaosUser(referrer).UUID(), Referee: referralChaosUser(referee).UUID(),
			CodeKind: "random", Source: "clipboard",
		}
	})
}

func TestReferralFollows_CrashBeforeCommit_RetryEndsInTheSameState(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	e := f.attributed(f.alice, f.bob)
	runs := 0
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		runs++
		if runs == 2 {
			if rows, created := f.followRows(ctx, t), f.createdEvents(ctx, t); len(rows) != 0 || len(created) != 0 {
				t.Fatalf("after the crash: follows %+v, follow.created %+v; want nothing committed", rows, created)
			}
		}
		return f.deliverIn(ctx, e)
	})
	want := wantRows("referral", [2]ids.UserID{f.alice, f.bob}, [2]ids.UserID{f.bob, f.alice})
	if got := f.followRows(t.Context(), t); !slices.Equal(got, want) {
		t.Fatalf("follows = %+v, want %+v", got, want)
	}
	if created := f.createdEvents(t.Context(), t); len(created) != 2 {
		t.Fatalf("follow.created = %+v, want 2", created)
	}
}
