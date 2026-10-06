//go:build faultpoints

package notify_test

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const chaosUsers, chaosTokenless, chaosDeleted = 4, 2, 3

func seedChaosUsers(t *testing.T, h testkit.Harness) {
	t.Helper()
	seeded := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i := range chaosUsers {
		status := "active"
		if i == chaosDeleted {
			status = "deleted"
		}
		if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
			(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at,
			 deleted_at)
			VALUES ($1, $2, 'sms', $3, $4, $3, $3, CASE WHEN $4 = 'deleted' THEN $3::timestamptz END)`,
			chaosRecipient(i), "did:privy:notify-chaos-"+strconv.Itoa(i), seeded, status); err != nil {
			t.Fatalf("seed user %d: %v", i, err)
		}
		if i == chaosTokenless {
			continue
		}
		if _, err := h.Pool.Exec(t.Context(), `INSERT INTO device_tokens
			(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, 'sandbox', $4, $4)`,
			uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-token-"+strconv.Itoa(i))), chaosRecipient(i),
			strings.Repeat(strconv.Itoa(i), 64), seeded); err != nil {
			t.Fatalf("seed token %d: %v", i, err)
		}
	}
}

func chaosCabal(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-cabal-"+strconv.Itoa(i)))
}

func chaosCabals() *fakes.Cabal {
	joined := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	seeds := []fakes.CabalSeed{
		{View: cabal.View{ID: ids.CabalIDFrom(chaosCabal(0)), Name: cabalName}},
		{View: cabal.View{ID: ids.CabalIDFrom(chaosCabal(1)), Name: "Rocket"}},
	}
	memberships := [][2]int{{0, 0}, {0, 1}, {0, chaosTokenless}, {1, 1}, {1, chaosDeleted}}
	rows := make([]fakes.CabalMember, 0, len(memberships))
	for _, m := range memberships {
		rows = append(rows, fakes.CabalMember{CabalID: ids.CabalIDFrom(chaosCabal(m[0])), Member: cabal.MemberView{
			UserID: ids.UserIDFrom(chaosRecipient(m[1])), Role: cabal.RoleMember, CanVote: true, JoinedAt: joined,
		}})
	}
	return fakes.NewCabal(seeds, rows)
}

func chaosScope(n int) *uuid.UUID {
	if n%3 == 2 {
		return nil
	}
	id := chaosCabal(n % 3)
	return &id
}

func chaosDeposit(i int) events.Event {
	user := (i / 2) % chaosUsers
	amounts := []uint64{1_250_000, 25_000_000, 9_999, 1_234_560_000}
	return events.DepositCredited{
		V: 1, DepositID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-deposit-"+strconv.Itoa(i))),
		UserID: chaosRecipient(user), WalletAddress: chain.SolanaAddress("wallet-" + strconv.Itoa(user)),
		AmountMicros: money.MicrosFromUint64(amounts[i%len(amounts)]),
		TxSignature:  chain.Signature("sig-" + strconv.Itoa(i)), Slot: int64(i),
	}
}

func TestNotify_DepositAndCabalPauseKinds_ConvergeUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		m := notify.New(deps, notify.WithSender(&testkit.FakeSender{}), notify.WithCabals(chaosCabals()))
		return m.Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event {
		scope := chaosScope(i / 4)
		switch i % 4 {
		case 1:
			return events.CabalPaused{
				V: 1, PauseID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-pause-"+strconv.Itoa(i))),
				CabalID: scope, Reason: "external_deposit", Scope: scopeOf(scope),
			}
		case 2:
			return events.CabalResumed{V: 1, CabalID: scope, Scope: scopeOf(scope)}
		default:
			return chaosDeposit(i)
		}
	})
}

func chaosFollow(i int) events.Event {
	round, followee := i/chaosUsers, i%chaosUsers
	return events.FollowCreated{
		V: 1, FollowID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-follow-"+strconv.Itoa(i))),
		FollowerID: chaosRecipient((followee + 1 + round) % chaosUsers), FolloweeID: chaosRecipient(followee),
		Source:    []string{"profile", "phone", "x", "cabal", "feed", "suggested", "referral"}[i%7],
		CreatedAt: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
	}
}

func TestNotify_FollowKind_ConvergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		return notify.New(deps, notify.WithSender(&testkit.FakeSender{})).Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosFollow(i) })
}
