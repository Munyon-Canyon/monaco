//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNames_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	b := seedBoards(t, d, true)
	before := d.entries(t)
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	ev := renamed(b.alice, "display_name", "handle", "photo_url")
	id := d.event(t, ev)

	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = d.deliverAs(faultpoint.Armed(t.Context(), faultpoint.BeforeCommit), t, id, namesHandler, ev)
		return nil
	}()
	if !faultpoint.IsCrash(crashed) {
		t.Fatalf("the delivery never crashed at before-commit, recovered %v", crashed)
	}
	if got := d.entries(t); !slices.Equal(got, before) || !slices.Equal(d.revs(t), []int{0, 0}) {
		t.Fatalf("after the crash entries = %q, revs %v; want nothing committed", got, d.revs(t))
	}
	wantNoHint(t, nats.Conn, sub)

	for _, wantDuplicate := range []bool{false, true} {
		duplicate, err := d.deliverAs(t.Context(), t, id, namesHandler, ev)
		if err != nil || duplicate != wantDuplicate {
			t.Fatalf("redelivery = duplicate %t, %v; want duplicate %t", duplicate, err, wantDuplicate)
		}
	}
	if got := d.entries(t); !slices.Equal(got, renamedBoards()) || !slices.Equal(d.revs(t), []int{0, 1}) {
		t.Fatalf("after redelivery entries = %q, revs %v; want the rename and one bump", got, d.revs(t))
	}
	wantHint(t, nats.Conn, sub, b)
}

func chaosUser(i int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("user"+strconv.Itoa(i))) }

func seedChaosBoards(t *testing.T, h testkit.Harness) {
	t.Helper()
	d := deliverer{pool: h.Pool, gen: h.IDs, clock: h.Clock}
	b := seedBoards(t, d, true)
	for i := range 12 {
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO leaderboard_entries (
			board, range, rank, subject_id, subject_name, subject_created_at, value_micros, pnl_micros,
			prices_as_of, computed_at
		) VALUES ('people', 'ALL', $1 + 10, $2, 'old', $3, 1, 0, $3, $3),
			('cabal_members:' || $4::text, 'ALL', $1 + 10, $2, 'old', $3, 1, 0, $3, $3)`,
			i, chaosUser(i), b.done, b.cabal)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestNames_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosBoards(t, h)
		return rankingConsumer(t, h, "ranking_names")
	}, func(_ *rand.Rand, i int) events.Event {
		field := "display_name"
		if i%4 == 3 {
			field = "bio"
		}
		return events.UserProfileUpdated{
			V: 1, UserID: chaosUser(i), Fields: []string{field}, DisplayName: "Name " + strconv.Itoa(i),
		}
	})
}
