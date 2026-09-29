package system_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withSystem() scenario.Option {
	return scenario.WithModules(func(d module.Deps) module.Module { return system.New(d) })
}

func TestSeed_oneUserWithPingReplaysThePingThroughTheEchoConsumer(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	consumers := system.New(module.Deps{Pool: f.pool, Clock: testkit.NewClock(f.now), Bus: testkit.NATS(t).Conn}).
		Consumers()
	seeded := testkit.Seed(t, f.pool, "one-user-with-ping", consumers...)
	if len(seeded) != 1 {
		t.Fatalf("seeded %d events, want 1", len(seeded))
	}
	ev, ok := seeded[0].Event.(events.SystemPinged)
	if !ok || seeded[0].Actor != "user:"+ev.UserID.String() {
		t.Fatalf("seeded %+v, want a system.pinged by its user", seeded[0])
	}
	user, err := ids.ParseUserID(ev.UserID.String())
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.GetPing(t.Context(), f.pool, ev.PingID, user)
	if err != nil || got != (app.Ping{ID: ev.PingID, Note: ev.Note, Echoed: true}) {
		t.Fatalf("GetPing after the seed = %+v, %v, want the seeded ping echoed", got, err)
	}
}

func TestScenario_readsASeededPingAsItsUser(t *testing.T) {
	t.Parallel()
	scenario.New(t, withSystem()).
		Given(scenario.Seeded("one-user-with-ping", "alice"), scenario.AsUser("alice")).
		When(scenario.Get("/v1/system/pings/{system.pinged}")).
		Then(scenario.ExpectStatus(http.StatusOK), scenario.ExpectJSON("echoed", true),
			scenario.ExpectJSON("note", "seeded"))
}
