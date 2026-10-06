//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHints_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return rankingConsumer(t, h, "ranking_hints")
	}, func(rng *rand.Rand, i int) events.Event {
		run := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		at := time.Date(2026, 10, 6, 12, 0, i, 0, time.UTC)
		return events.RankingSnapshotWritten{V: 1, RunID: run, AsOf: at, PricesAsOf: at, ComputedAt: at, RowsWritten: i}
	})
}
