//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestFeedConsumer_Redelivery_NoDuplicate(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		users := fakes.NewIdentity(nil, nil)
		for _, c := range social.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}, social.WithUsers(users)).Consumers() {
			if c.Durable == "social_feed" {
				return c
			}
		}
		t.Fatal("social_feed is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		return events.CabalCreated{
			V: 1, CabalID: uuid.NewSHA1(uuid.Nil, []byte(strconv.Itoa(i))),
			CreatorID: uuid.NewSHA1(uuid.Nil, []byte("creator"+strconv.Itoa(i))), Name: "Alpha",
		}
	})
}
