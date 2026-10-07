//go:build faultpoints

package referrals_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const qualifyChaosReferees = 12

func qualifyChaosReferee(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, strconv.AppendInt([]byte("referee-"), int64(i), 10))
}

func seedQualifyReferees(t *testing.T, h testkit.Harness) *fakes.Identity {
	t.Helper()
	cards := make([]identity.UserCard, qualifyChaosReferees)
	for i := range cards {
		referee := qualifyChaosReferee(i)
		cards[i] = identity.UserCard{
			ID: ids.UserIDFrom(referee), AccountStatus: identity.AccountActive, PhoneVerified: true,
		}
		if _, err := h.Pool.Exec(t.Context(),
			`INSERT INTO referrals (id, referrer_id, referee_id, code, code_kind, source, status, created_at)
			VALUES ($1, $2, $3, 'k7m4qx2p', 'random', 'manual', 'attributed', $4)`,
			h.IDs.NewV7(), h.IDs.NewV7(), referee, deliveredAt(),
		); err != nil {
			t.Fatal(err)
		}
	}
	return fakes.NewIdentity(cards, nil)
}

func qualifyDurable(t *testing.T, h testkit.Harness, users *fakes.Identity) bus.Consumer {
	t.Helper()
	for _, c := range referrals.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
		if c.Durable != "referrals" {
			continue
		}
		for i, spec := range c.Handlers {
			if spec.Name == "referrals.qualify" {
				c.Handlers[i] = bus.Handle("referrals.qualify", adapters.Qualify{Users: users}.Handle)
			}
		}
		return c
	}
	t.Fatal("the referrals durable is not registered")
	return bus.Consumer{}
}

func TestQualify_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return qualifyDurable(t, h, seedQualifyReferees(t, h))
	}, func(rng *rand.Rand, i int) events.Event {
		return events.Funded{
			V: 1, TransferID: uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)),
			CabalID: uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)),
			UserID:  qualifyChaosReferee(i), AmountMicros: money.MicrosFromUint64(10_000_000),
		}
	})
}
