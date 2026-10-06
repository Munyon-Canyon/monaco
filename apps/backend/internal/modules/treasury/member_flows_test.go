package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type flowsCase struct {
	from, to time.Time
	want     []port.MemberFlow
}

func flowsFixture(t *testing.T, f fixture) flowsCase {
	t.Helper()
	user, other, cabal, otherCabal := f.user(t), f.user(t), f.cabal(t), f.cabal(t)
	net := money.SignedMicrosFromInt64
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	from := f.clock.Now()
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, user, cabal, 50_000_000, 50)
	first := port.MemberFlow{UserID: user, CabalID: cabal, Amount: net(50_000_000), At: f.clock.Now()}
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, other, otherCabal, 7_000_000, 7)
	second := port.MemberFlow{UserID: other, CabalID: otherCabal, Amount: net(7_000_000), At: f.clock.Now()}
	f.clock.Advance(time.Second)
	mustCashOutHistory(t, f, user, cabal, 25_000_000, 25)
	third := port.MemberFlow{UserID: user, CabalID: cabal, Amount: net(-25_000_000), At: f.clock.Now()}
	to := f.clock.Now()
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, user, cabal, 1_000_000, 1)
	return flowsCase{from: from, to: to, want: []port.MemberFlow{first, second, third}}
}

func assertFlows(t *testing.T, reader port.HistoricalReader, c flowsCase) {
	t.Helper()
	got, err := reader.MemberFlowsBetween(t.Context(), c.from, c.to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(c.want) {
		t.Fatalf("MemberFlowsBetween = %#v, want %#v", got, c.want)
	}
	for i, flow := range got {
		w := c.want[i]
		if flow.UserID != w.UserID || flow.CabalID != w.CabalID || flow.Amount != w.Amount || !flow.At.Equal(w.At) {
			t.Fatalf("flow %d = %#v, want %#v", i, flow, w)
		}
	}
	none, err := reader.MemberFlowsBetween(t.Context(), c.to, c.to)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty window = %#v, %v", none, err)
	}
}

func TestMemberFlowsBetween_AdapterAndFakeShareAContract(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := flowsFixture(t, f)
	t.Run("adapter", func(t *testing.T) {
		t.Parallel()
		assertFlows(t, newQueries(f), c)
	})
	t.Run("fake", func(t *testing.T) {
		t.Parallel()
		fake := fakes.NewTreasury()
		fake.SetMemberFlows(append([]port.MemberFlow{
			{Amount: money.SignedMicrosFromInt64(100_000_000), At: c.from},
			{Amount: money.SignedMicrosFromInt64(1_000_000), At: c.to.Add(time.Second)},
		}, c.want...))
		assertFlows(t, fake, c)
	})
}

func TestMemberFlowsBetween_OneQuery(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := flowsFixture(t, f)
	q := newQueries(f)
	testkit.AssertQueries(t, "MemberFlowsBetween", func() {
		if _, err := q.MemberFlowsBetween(t.Context(), c.from, c.to); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMemberFlowsBetween_CanceledReadFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := newQueries(newFixture(t))
	if _, err := q.MemberFlowsBetween(ctx, time.Time{}, time.Unix(1, 0)); err == nil {
		t.Fatal("MemberFlowsBetween error = nil")
	}
}

func TestMemberFlowsBetween_FakeFault(t *testing.T) {
	t.Parallel()
	fake := fakes.NewTreasury()
	fake.Fail("MemberFlowsBetween", errs.New(errs.CodeInternal, "test"))
	if _, err := fake.MemberFlowsBetween(t.Context(), time.Time{}, time.Unix(1, 0)); err == nil {
		t.Fatal("MemberFlowsBetween error = nil")
	}
}
