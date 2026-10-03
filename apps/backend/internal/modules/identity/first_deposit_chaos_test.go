//go:build faultpoints

package identity_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func chaosDepositor(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("first-deposit-chaos-user-"+strconv.Itoa(i)))
}

func TestFirstDeposit_convergesUnderChaosWithTwoQualifyingDepositsPerUserAtOneBlockTime(t *testing.T) {
	t.Parallel()
	const users, deletedUser = 4, 3
	created := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for i := range users {
			status := "active"
			if i == deletedUser {
				status = "deleted"
			}
			if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
				(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at, deleted_at)
				VALUES ($1, $2, 'sms', $3, $4, $3, $3, CASE WHEN $4 = 'deleted' THEN $3::timestamptz END)`,
				chaosDepositor(i), "did:privy:chaos-"+strconv.Itoa(i), created, status); err != nil {
				t.Fatalf("seed user %d: %v", i, err)
			}
		}
		for _, c := range identity.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "identity_first_deposit" {
				return c
			}
		}
		t.Fatal("identity_first_deposit is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, i int) events.Event {
		user := i % users
		qualifyingTwiceThenBelow := []uint64{10_000_000, 50_000_000, 9_999_999}
		amount := qualifyingTwiceThenBelow[i/users]
		sameBlockForBothQualifying := created.Add(time.Duration(user+1) * time.Hour)
		return events.DepositCredited{
			V: 1, DepositID: uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)),
			UserID: chaosDepositor(user), WalletAddress: chain.SolanaAddress("wallet-" + strconv.Itoa(user)),
			AmountMicros: money.MicrosFromUint64(amount), TxSignature: chain.Signature("sig-" + strconv.Itoa(i)),
			Slot: int64(i), BlockTime: &sameBlockForBothQualifying,
		}
	})
}
