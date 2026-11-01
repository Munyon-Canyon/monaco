package notify_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

func (r *pushRig) deliver(t *testing.T, e events.Event) func() {
	t.Helper()
	conn := testkit.NATS(t).Conn
	consumer := bus.Consumer{Durable: adapters.DisableDevicesDurable, Handlers: adapters.DisableDevicesHandlers()}
	reg, err := bus.NewRegistry(conn, r.uow, r.clock, []bus.Consumer{consumer})
	if err != nil {
		t.Fatal(err)
	}
	d := r.emit(t, "system:disable-devices-test", e)
	var payload []byte
	if err := r.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE id = $1`, d.EventID.UUID()).
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	return func() {
		reg.Dispatch(t.Context(), consumer.Durable, chaos.NewMsg(conn, e.Type(), d.EventID, payload))
	}
}

func (r *pushRig) wantActive(t *testing.T, user ids.UserID, want int) {
	t.Helper()
	r.wantCount(t, "active device tokens", want,
		`SELECT count(*) FROM device_tokens WHERE user_id = $1 AND disabled_at IS NULL`, user.UUID())
}

func TestDisableDevices_endsEveryDeviceOfAUserWhoRevokedAllSessionsAndIgnoresARedelivery(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	leaver, other := r.user(t, "active"), r.user(t, "active")
	for _, tok := range []string{"aaaa" + "1", "aaaa" + "2"} {
		r.device(t, leaver, tok)
	}
	r.device(t, other, "bbbb1")
	deliver := r.deliver(t, events.UserSessionsRevoked{
		V: 1, UserID: leaver.UUID(), Scope: "all_sessions", At: r.clock.Now(),
	})
	r.clock.Advance(time.Minute)
	deliver()
	r.wantActive(t, leaver, 0)
	r.wantActive(t, other, 1)
	r.device(t, leaver, "aaaa3")
	r.clock.Advance(time.Minute)
	deliver()
	r.wantActive(t, leaver, 1)
	r.wantCount(t, "recorded deliveries", 1,
		`SELECT count(*) FROM event_deliveries WHERE handler = 'notify.disable_devices_sessions_revoked'`)
}

func TestDisableDevices_endsEveryDeviceOfADeletedAccountAndIgnoresARedelivery(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	gone, other := r.user(t, "active"), r.user(t, "active")
	r.device(t, gone, "cccc1")
	r.device(t, gone, "cccc2")
	r.device(t, other, "dddd1")
	deliver := r.deliver(t, events.UserDeleted{V: 1, UserID: gone.UUID(), At: r.clock.Now()})
	deliver()
	r.wantActive(t, gone, 0)
	r.wantActive(t, other, 1)
	r.device(t, gone, "cccc3")
	deliver()
	r.wantActive(t, gone, 1)
	r.wantCount(t, "recorded deliveries", 1,
		`SELECT count(*) FROM event_deliveries WHERE handler = 'notify.disable_devices_user_deleted'`)
}
