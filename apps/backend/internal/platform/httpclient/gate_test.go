package httpclient_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

func TestDo_aGateAdmitsEveryAttemptTheClientSends(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusServiceUnavailable), netErr, status(http.StatusOK)}}
		c := client(u, httpclient.WithRetry(3, time.Millisecond, time.Millisecond))
		var admitted atomic.Int32
		ctx := httpclient.WithAttemptGate(t.Context(), func(context.Context) error {
			admitted.Add(1)
			return nil
		})
		if resp := mustOK(ctx, t, c, get(t, "/v1/users")); resp.status != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.status)
		}
		if admitted.Load() != 3 || u.count() != 3 {
			t.Fatalf("admitted %d attempts and sent %d, want 3 of each", admitted.Load(), u.count())
		}
	})
}

func TestDo_aGateThatRefusesTheFirstAttemptSendsNothingAndKeepsItsError(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{status(http.StatusOK)}}
	c := client(u, httpclient.WithRetry(3, time.Millisecond, time.Millisecond))
	ctx := httpclient.WithAttemptGate(t.Context(), func(context.Context) error {
		return errs.New(errs.CodeCoinGeckoRateLimited, "test.gate")
	})
	err := mustFail(ctx, t, c, get(t, "/v1/users"))
	if errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited || u.count() != 0 {
		t.Fatalf("err = %v after %d calls, want the gate's error and no call", err, u.count())
	}
}

func TestDo_aGateThatRefusesARetryReturnsTheFailureThatNeededIt(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusTooManyRequests), status(http.StatusOK)}}
		c := client(u, httpclient.WithRetry(3, time.Millisecond, time.Millisecond))
		var admitted atomic.Int32
		ctx := httpclient.WithAttemptGate(t.Context(), func(context.Context) error {
			if admitted.Add(1) > 1 {
				return errs.New(errs.CodeCoinGeckoRateLimited, "test.gate")
			}
			return nil
		})
		err := mustFail(ctx, t, c, get(t, "/v1/users"))
		wantFailure(t, err, errs.CodeUpstreamUnavailable, http.StatusTooManyRequests, 1)
		if u.count() != 1 || admitted.Load() != 2 {
			t.Fatalf("sent %d calls after %d admissions, want 1 call after 2", u.count(), admitted.Load())
		}
	})
}

func TestDo_anOpenBreakerNeverReachesTheGate(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{netErr}}
		c := client(u, httpclient.WithRetry(5, time.Second, time.Second), httpclient.WithBreaker(gobreaker.Settings{
			ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 2 },
		}))
		var admitted atomic.Int32
		ctx := httpclient.WithAttemptGate(t.Context(), func(context.Context) error {
			admitted.Add(1)
			return nil
		})
		_ = mustFail(ctx, t, c, get(t, "/v1/users"))
		if admitted.Load() != 2 || u.count() != 2 {
			t.Fatalf("admitted %d and sent %d, want 2 of each: the refused third attempt takes no slot",
				admitted.Load(), u.count())
		}
	})
}

func TestDo_theCallTimeoutStartsOnceTheFirstAttemptIsAdmitted(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusOK)}}
		c := client(u, httpclient.WithTimeout(10*time.Second))
		ctx := httpclient.WithAttemptGate(t.Context(), func(context.Context) error {
			<-time.After(time.Minute)
			return nil
		})
		if resp := mustOK(ctx, t, c, get(t, "/v1/users")); resp.status != http.StatusOK {
			t.Fatalf("status = %d, want 200 after a wait longer than the timeout", resp.status)
		}
	})
}

func TestDo_aCallerThatGivesUpWhileGatedSendsNothing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{status(http.StatusOK)}}
		c := client(u)
		gated := httpclient.WithAttemptGate(t.Context(), func(ctx context.Context) error {
			<-ctx.Done()
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamUnavailable, "test.gate")
		})
		ctx, cancel := context.WithTimeout(gated, time.Second)
		defer cancel()
		err := mustFail(ctx, t, c, get(t, "/v1/users"))
		if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || u.count() != 0 {
			t.Fatalf("err = %v after %d calls, want unavailable and no call", err, u.count())
		}
	})
}
