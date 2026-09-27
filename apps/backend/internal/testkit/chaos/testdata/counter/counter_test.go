//go:build faultpoints

package counter_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithNATS())
}

func TestCounterWithoutDedupe(t *testing.T) {
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		if _, err := h.Pool.Exec(t.Context(), `CREATE TABLE counter_hits (ping_id uuid NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		count := func(ctx context.Context, _ db.Tx, e events.SystemPinged) error {
			_, err := h.Pool.Exec(ctx, `INSERT INTO counter_hits (ping_id) VALUES ($1)`, e.PingID)
			return err
		}
		return bus.Consumer{Durable: "counter", Handlers: []bus.HandlerSpec{bus.Handle("counter.count", count)}}
	}, func(rng *rand.Rand, i int) events.Event {
		return events.SystemPinged{V: 1, PingID: uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)), Note: strconv.Itoa(i)}
	})
}
