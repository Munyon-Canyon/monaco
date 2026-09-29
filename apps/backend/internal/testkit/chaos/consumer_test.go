//go:build faultpoints

package chaos_test

import (
	"context"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func ping(rng *rand.Rand, i int) events.Event {
	return events.SystemPinged{
		V: 1, PingID: uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)), Note: strconv.Itoa(i),
	}
}

func TestConsumerSuite_aDedupedConsumerConvergesOnEverySeed(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		_, err := h.Pool.Exec(t.Context(), `CREATE TABLE counter_hits (id uuid PRIMARY KEY, ping_id uuid NOT NULL)`)
		if err != nil {
			t.Fatal(err)
		}
		count := func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
			_, err := tx.Queries().Exec(ctx,
				`INSERT INTO counter_hits (id, ping_id) VALUES ($1, $2)`, h.IDs.NewV7(), e.PingID)
			return err
		}
		echo := func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
			return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: h.IDs.NewV7(), Note: "echo " + e.Note})
		}
		return bus.Consumer{Durable: "counter", Handlers: []bus.HandlerSpec{
			bus.Handle("counter.count", count), bus.Handle("counter.echo", echo),
		}}
	}, ping)
}

func goTestChaos(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs another test binary; CI runs it without -short, outside the 10 s package budget")
	}
	cmd := exec.CommandContext(t.Context(), "go", append([]string{
		"test", "-tags", "faultpoints", "-count=1", "-v", "-run", "^TestCounterWithoutDedupe$", "./testdata/counter",
	}, args...)...)
	cmd.Env = append(os.Environ(), "CHAOS_SEEDS=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestConsumerSuite_aCounterWithoutDedupeFailsWithASeedThatReplays(t *testing.T) {
	t.Parallel()
	out, err := goTestChaos(t)
	m := regexp.MustCompile(`chaos seed (\d+) diverged`).FindStringSubmatch(out)
	if err == nil || m == nil || !strings.Contains(out, "+ public.counter_hits ") ||
		!strings.Contains(out, "rerun with -chaos.seed="+m[1]) {
		t.Fatalf("non-idempotent counter did not fail with a printed seed (err %v):\n%s", err, out)
	}
	replay, err := goTestChaos(t, "-args", "-chaos.seed="+m[1])
	ran := regexp.MustCompile(`=== RUN   TestCounterWithoutDedupe/seed=(\d+)`).FindAllStringSubmatch(replay, -1)
	if err == nil || len(ran) != 1 || ran[0][1] != m[1] || !strings.Contains(replay, "chaos seed "+m[1]+" diverged") {
		t.Fatalf("-chaos.seed=%s did not replay the failure:\n%s", m[1], replay)
	}
}
