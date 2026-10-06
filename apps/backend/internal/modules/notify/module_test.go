package notify_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

func TestModule_servesDevicesAndConsumesTestPushRequestsWithNoPollers(t *testing.T) {
	t.Parallel()
	m := notify.New(module.Deps{})
	var handlers []string
	for _, c := range m.Consumers() {
		for _, h := range c.Handlers {
			handlers = append(handlers, c.Durable+" "+h.Name+" "+string(h.Type()))
		}
	}
	if m.Name() != "notify" || !testkit.Serves(m.Mount, "POST", "/v1/devices") ||
		!testkit.Serves(m.Mount, "DELETE", "/v1/devices/"+token('a')) || m.Pollers() != nil ||
		!slices.Equal(handlers, []string{"notify notify.notify_test_requested notify.test_requested"}) {
		t.Fatalf("module = %s, handlers %q, pollers %v", m.Name(), handlers, m.Pollers())
	}
}

func TestModule_sendsThroughTheChosenSender(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		options              func(d *module.Deps, chosen *testkit.FakeSender) []notify.Option
		wantDeps, wantChosen int
	}{
		"deps.APNs": {func(*module.Deps, *testkit.FakeSender) []notify.Option { return nil }, 1, 0},
		"WithSender": {func(_ *module.Deps, chosen *testkit.FakeSender) []notify.Option {
			return []notify.Option{notify.WithSender(chosen)}
		}, 0, 1},
		"noop when none is bound": {func(d *module.Deps, _ *testkit.FakeSender) []notify.Option {
			d.APNs = nil
			return []notify.Option{notify.WithSender(nil)}
		}, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			user := r.user(t, "active")
			r.device(t, user, token('a'))
			bound, chosen := &testkit.FakeSender{}, &testkit.FakeSender{}
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock, APNs: bound}
			opts := tc.options(&deps, chosen)

			d := dispatch(t, r, notify.New(deps, opts...), user)

			if len(bound.Sent()) != tc.wantDeps || len(chosen.Sent()) != tc.wantChosen {
				t.Fatalf("deps.APNs sent %d and the option's sender %d, want %d and %d",
					len(bound.Sent()), len(chosen.Sent()), tc.wantDeps, tc.wantChosen)
			}
			r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
			r.wantRecorded(t, d, 1)
		})
	}
}

func dispatch(t *testing.T, r *pushRig, m *notify.Module, user ids.UserID) bus.Delivery {
	t.Helper()
	conn := testkit.NATS(t).Conn
	reg, err := bus.NewRegistry(conn, r.uow, r.clock, m.Consumers())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := r.trigger(t, opsActor, user)
	var payload []byte
	if err := r.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE id = $1`, d.EventID.UUID()).
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	reg.Dispatch(t.Context(), "notify", chaos.NewMsg(conn, events.TypeNotifyTestRequested, d.EventID, payload))
	return d
}
