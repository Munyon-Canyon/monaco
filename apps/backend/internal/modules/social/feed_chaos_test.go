//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const chaosCabals = 6

func chaosCabal(i int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(strconv.Itoa(i))) }

func chaosCreator(i int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("creator"+strconv.Itoa(i))) }

func seedStaleFeed(t *testing.T, h testkit.Harness) {
	t.Helper()
	for c := range chaosCabals {
		for p := range chaosCabals {
			payload := feed.Payload{CabalName: "Old " + strconv.Itoa(c), ActorName: "Old " + strconv.Itoa(p)}
			_, err := h.Pool.Exec(t.Context(), `INSERT INTO feed_objects
				(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, title, payload, created_at, updated_at)
				VALUES ($1, 'member_joined', 'cabal_members', $2, $3, $4, $5, $6, $7, now(), now())`,
				h.IDs.NewV7(), h.IDs.NewV7(), chaosCabal(c), payload.CabalName, chaosCreator(p),
				feed.RenderTitle(feed.KindMemberJoined, payload), payload.JSON())
			if err != nil {
				t.Fatalf("seed item %d/%d: %v", c, p, err)
			}
		}
	}
}

func chaosFeedConsumer(t *testing.T, h testkit.Harness) bus.Consumer {
	t.Helper()
	cards := make([]identity.UserCard, chaosCabals)
	for p := range chaosCabals {
		card := identity.UserCard{ID: ids.UserIDFrom(chaosCreator(p))}
		card.Handle, card.DisplayName = "p"+strconv.Itoa(p), "Renamed "+strconv.Itoa(p)
		cards[p] = card
	}
	deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
	for _, c := range social.New(deps, social.WithUsers(fakes.NewIdentity(cards, nil)), social.WithCabals(chaosNames())).Consumers() {
		if c.Durable == "social_feed" {
			return c
		}
	}
	t.Fatal("social_feed is not registered")
	return bus.Consumer{}
}

func chaosNames() *cabalNames {
	return &cabalNames{current: map[ids.CabalID]string{
		ids.CabalIDFrom(chaosCabal(1)): "Renamed 1", ids.CabalIDFrom(chaosCabal(5)): "Renamed 5",
	}}
}

func chaosEvent(_ *rand.Rand, i int) events.Event {
	picture := "https://cdn.example.com/p.png"
	renames := []events.Event{
		profileUpdated(chaosCreator(0), "", "display_name"),
		cabalRenamed(chaosCabal(1), chaosCreator(0), "Renamed 1"),
		profileUpdated(chaosCreator(2), "", "photo"),
		events.CabalUpdated{
			V: 1, CabalID: chaosCabal(3), ActorID: chaosCreator(3), Changes: events.CabalChanges{PictureURL: &picture},
		},
		profileUpdated(chaosCreator(4), "", "display_name", "handle"),
		cabalRenamed(chaosCabal(5), chaosCreator(5), "Renamed 5"),
	}
	if i >= chaosCabals {
		return renames[i-chaosCabals]
	}
	return events.CabalCreated{V: 1, CabalID: chaosCabal(i), CreatorID: chaosCreator(i), Name: "Alpha"}
}

func TestFeedConsumer_Redelivery_NoDuplicate(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedStaleFeed(t, h)
		return chaosFeedConsumer(t, h)
	}, chaosEvent)
}
