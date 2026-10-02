//go:build faultpoints

package referrals_test

import (
	"context"
	"io"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func entropyPerEvent(ctx context.Context) io.Reader {
	var seed [32]byte
	copy(seed[:], observability.EventIDFrom(ctx))
	return rand.NewChaCha8(seed)
}

func TestMintCode_mintsOneCodePerUserUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, c := range referrals.New(deps, referrals.WithEntropy(entropyPerEvent)).Consumers() {
			if c.Durable == "referrals" {
				return c
			}
		}
		t.Fatal("the referrals durable is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, _ int) events.Event {
		user := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		return events.UserCreated{V: 1, UserID: user, LoginProvider: "sms", CreatedAt: deliveredAt()}
	})
}
