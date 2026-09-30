//go:build faultpoints

package analytics_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/testdata"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

const chaosEvents = 4

func TestAnalytics_Probe_ExportsEachEventOnceUnderChaos(t *testing.T) {
	t.Parallel()
	for _, seed := range chaos.Seeds(t) {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, testdata.Register)
			want := map[uuid.UUID]bool{}
			msgs := make([]*chaos.Msg, 0, chaosEvents)
			for range chaosEvents {
				a := e.append(t, userActor(e.ids.NewV7()))
				want[a.id] = true
				msgs = append(msgs, chaos.NewMsg(e.bus.Conn, events.TypeSystemPinged, ids.EventIDFrom(a.id), a.payload))
			}
			res := chaos.Dispatch(e.ctx(t), t, seed, e.reg, e.consumer, msgs)
			captures := e.fake.Captures()
			if len(captures) != chaosEvents || len(res.Terms()) != 0 {
				t.Fatalf("seed %d: %d distinct captures and terms %v, want %d and none\n%v",
					seed, len(captures), res.Terms(), chaosEvents, res.Trace)
			}
			for _, c := range captures {
				if !want[c.UUID] {
					t.Errorf("seed %d: capture for %s, which was never appended", seed, c.UUID)
				}
			}
		})
	}
}
