package funding_test

import (
	"crypto/ed25519"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const freshCap = 2

func freshDestination(n int) chain.SolanaAddress {
	return chain.AddressOf(fakes.FixtureKey(fmt.Sprintf("fresh-destination-%d", n)).Public().(ed25519.PublicKey))
}

func capped(d *app.WithdrawDeps) { d.FreshDestinationsPerDay = freshCap }

func (f *withdrawFixture) fillFreshDestinations(t *testing.T) {
	t.Helper()
	for n := range freshCap {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); err != nil {
			t.Fatalf("withdraw to fresh destination %d: %v", n, err)
		}
	}
}

func (f *withdrawFixture) assertRefusedBeforeSigning(t *testing.T, to chain.SolanaAddress) {
	t.Helper()
	builds := len(f.transfers.builds)
	_, err := f.handle(f.ctx(t.Context()), to)
	if errs.CodeOf(err) != errs.CodeRateLimited {
		t.Fatalf("Handle err = %v, want rate_limited", err)
	}
	if len(f.transfers.builds) != builds || len(f.transfers.sent) != builds {
		t.Fatalf("builds = %d sent = %d, want %d of each", len(f.transfers.builds), len(f.transfers.sent), builds)
	}
}

func TestWithdraw_CapsFreshDestinationsPerDay(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, capped)
	f.fillFreshDestinations(t)
	f.assertRefusedBeforeSigning(t, freshDestination(freshCap))
	if got := len(f.rows(t)); got != freshCap {
		t.Fatalf("withdrawal rows = %d, want %d", got, freshCap)
	}
	if got := len(f.submittedEvents(t)); got != freshCap {
		t.Fatalf("submitted events = %d, want %d", got, freshCap)
	}
}

func TestWithdraw_RepeatDestinationIsNotCounted(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, capped)
	f.fillFreshDestinations(t)
	for range 3 {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(0)); err != nil {
			t.Fatalf("repeat to a known destination: %v", err)
		}
	}
	f.assertRefusedBeforeSigning(t, freshDestination(freshCap))
}

func TestWithdraw_FreshDestinationsAreCountedByTheDay(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, capped)
	f.fillFreshDestinations(t)
	f.clock.Advance(25 * time.Hour)
	for n := freshCap; n < 2*freshCap; n++ {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); err != nil {
			t.Fatalf("withdraw to fresh destination %d the next day: %v", n, err)
		}
	}
	f.assertRefusedBeforeSigning(t, freshDestination(2*freshCap))
}

func TestWithdraw_FailedAttemptsDoNotUseTheCap(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, capped)
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	for n := range 2 * freshCap {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); errs.CodeOf(err) != errs.CodePrivyUnavailable {
			t.Fatalf("Handle err = %v, want privy_unavailable", err)
		}
	}
	f.transfers.buildErr = nil
	f.fillFreshDestinations(t)
}

func TestWithdraw_DefaultCapAppliesWhenUnset(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000)
	for n := range app.FreshDestinationsPerDay {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); err != nil {
			t.Fatalf("withdraw to fresh destination %d: %v", n, err)
		}
	}
	f.assertRefusedBeforeSigning(t, freshDestination(app.FreshDestinationsPerDay))
}

func TestHTTPWithdraw_IsRateLimitedPerActor(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	member := flows.SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(
			flows.AuthorizeWithdrawal(`{"amount_micros":"2000000","to_address":"`+string(withdrawTo)+`"}`),
			flows.WithdrawAuthorized(`{"amount_micros":"2000000","to_address":"`+string(withdrawTo)+`"}`),
			scenario.ExpectStatus(http.StatusAccepted),
			drainRateLimitBuckets(),
			scenario.Post("/v1/me/withdrawals", `{"amount_micros":"2000000","to_address":"`+string(withdrawTo)+`"}`),
		).
		Then(
			scenario.ExpectProblem(errs.CodeRateLimited),
			scenario.ExpectStatus(http.StatusTooManyRequests),
			scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 1),
		)
}

func drainRateLimitBuckets() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		_, err := s.DB().Exec(s.Context(),
			`UPDATE rate_limit_buckets SET tokens_milli = 0, updated_at = now() + interval '1 hour'`)
		if err != nil {
			s.Fatalf("drain rate limit buckets: %v", err)
		}
	}
}

func TestWithdraw_FailsClosedWhenTheDestinationCountFails(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, capped)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE withdrawals RENAME COLUMN to_address TO to_addr`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.handle(f.ctx(t.Context()), freshDestination(0)); err == nil || len(f.transfers.builds) != 0 {
		t.Fatalf("Handle err = %v builds = %d, want an error and no signature", err, len(f.transfers.builds))
	}
}
