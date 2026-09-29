//go:build faultpoints

package system_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestEcho_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for _, c := range system.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "system_echo" {
				return c
			}
		}
		t.Fatal("system_echo is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, i int) events.Event {
		id := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		user := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		return events.SystemPinged{V: 1, PingID: id, UserID: user, Note: strconv.Itoa(i)}
	})
}
