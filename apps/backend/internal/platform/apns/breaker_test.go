package apns_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func TestSend_fiveConsecutiveServerErrorsOpenTheBreakerForThatEnvironmentOnly(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{
		rejected(http.StatusInternalServerError, "InternalServerError"),
		rejected(http.StatusInternalServerError, "InternalServerError"),
		rejected(http.StatusInternalServerError, "InternalServerError"),
		rejected(http.StatusInternalServerError, "InternalServerError"),
		rejected(http.StatusInternalServerError, "InternalServerError"),
		accepted(),
	}}
	c := newClient(t, u)
	for range 5 {
		if res := mustSend(t, c, push(apns.Sandbox)); res.Status != http.StatusInternalServerError {
			t.Fatalf("Send = %+v, want the 500 handed back for the caller to classify", res)
		}
	}

	_, err := c.Send(t.Context(), push(apns.Sandbox))

	wantCode(t, err, errs.CodeAPNSUnavailable)
	if !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 5 {
		t.Fatalf("err = %v after %d requests, want the open breaker to refuse without a request", err, u.count())
	}
	if res := mustSend(t, c, push(apns.Production)); res.Status != http.StatusOK || u.count() != 6 {
		t.Fatalf(
			"production Send = %+v after %d requests, want a sandbox outage to leave production open",
			res,
			u.count(),
		)
	}
}

func TestSend_aTrippedBreakerHalfOpensAfterItsTimeoutAndClosesOnSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{
			rejected(http.StatusServiceUnavailable, "ServiceUnavailable"),
			rejected(http.StatusServiceUnavailable, "ServiceUnavailable"),
			accepted(),
		}}
		c := newClient(t, u, apns.WithBreaker(gobreaker.Settings{
			Timeout:     45 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 2 },
		}))
		mustSend(t, c, push(apns.Sandbox))
		mustSend(t, c, push(apns.Sandbox))
		if _, err := c.Send(t.Context(), push(apns.Sandbox)); !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("err = %v, want the breaker open after two failures", err)
		}

		<-time.After(31 * time.Second)

		if _, err := c.Send(t.Context(), push(apns.Sandbox)); !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("err = %v, want the caller's 45s timeout to hold past the default 30s", err)
		}

		<-time.After(15 * time.Second)

		if res := mustSend(t, c, push(apns.Sandbox)); res.Status != http.StatusOK {
			t.Fatalf("half-open probe = %+v, want it to reach APNs and succeed", res)
		}
		if res := mustSend(t, c, push(apns.Sandbox)); res.Status != http.StatusOK || u.count() != 4 {
			t.Fatalf("Send = %+v after %d requests, want the breaker closed again", res, u.count())
		}
	})
}

func TestSend_aTrippedBreakerProbesAgainAfter30SecondsByDefault(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		replies := make([]reply, 0, 6)
		for range 5 {
			replies = append(replies, rejected(http.StatusInternalServerError, "InternalServerError"))
		}
		u := &upstream{replies: append(replies, accepted())}
		c := newClient(t, u)
		for range 5 {
			mustSend(t, c, push(apns.Sandbox))
		}

		<-time.After(29 * time.Second)

		if _, err := c.Send(t.Context(), push(apns.Sandbox)); !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("err = %v at 29s, want the breaker still open", err)
		}

		<-time.After(2 * time.Second)

		if res := mustSend(t, c, push(apns.Sandbox)); res.Status != http.StatusOK || u.count() != 6 {
			t.Fatalf("probe at 31s = %+v after %d requests, want it to reach APNs", res, u.count())
		}
	})
}

func TestSend_answersThatBlameTheTokenOrTheKeyNeverOpenTheBreaker(t *testing.T) {
	t.Parallel()
	for status, reason := range map[int]string{
		http.StatusTooManyRequests: "TooManyRequests",
		http.StatusForbidden:       "InvalidProviderToken",
		http.StatusBadRequest:      "BadTopic",
		http.StatusGone:            "Unregistered",
	} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{rejected(status, reason)}}
			c := newClient(t, u)
			for range 8 {
				if res := mustSend(t, c, push(apns.Sandbox)); res.Status != status {
					t.Fatalf("Send = %+v, want %d", res, status)
				}
			}
			if u.count() != 8 {
				t.Fatalf("%d requests reached APNs, want all 8", u.count())
			}
		})
	}
}

func TestSend_fiveHungCallsOpenTheBreakerBecauseTheDeadlineIsAPNsFault(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{replies: []reply{hang}}
		c := newClient(t, u)
		for range 5 {
			_, err := c.Send(t.Context(), push(apns.Sandbox))
			wantCode(t, err, errs.CodeAPNSUnavailable)
		}

		_, err := c.Send(t.Context(), push(apns.Sandbox))

		if !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 5 {
			t.Fatalf("err = %v after %d requests, want the sixth refused by the open breaker", err, u.count())
		}
	})
}

func TestSend_aCallersCancellationIsNotAPNsFault(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{hang, hang, hang, hang, hang, hang, accepted()}}
	c := newClient(t, u)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for range 6 {
		_, err := c.Send(ctx, push(apns.Sandbox))
		wantCode(t, err, errs.CodeAPNSUnavailable)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want the cancellation in the chain", err)
		}
	}

	if res := mustSend(t, c, push(apns.Sandbox)); res.Status != http.StatusOK {
		t.Fatalf("Send = %+v, want six cancelled callers to leave the breaker closed", res)
	}
}

func TestWithBreaker_keepsTheCallersSettingsAlongsideTheCancellationRule(t *testing.T) {
	t.Parallel()
	callerExcluded := errs.New(errs.CodeInternal, "test.excluded")
	u := &upstream{replies: []reply{
		func(*http.Request) (*http.Response, error) { return nil, callerExcluded },
		hang,
		unreachable,
	}}
	c := newClient(t, u, apns.WithBreaker(gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
		IsExcluded:  func(err error) bool { return errors.Is(err, callerExcluded) },
	}))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	for i, ctx := range []context.Context{t.Context(), cancelled} {
		if _, err := c.Send(ctx, push(apns.Sandbox)); errs.CodeOf(err) != errs.CodeAPNSUnavailable {
			t.Fatalf("send %d = %v, want unavailable without tripping the breaker", i, err)
		}
	}
	_, err := c.Send(t.Context(), push(apns.Sandbox))
	wantCode(t, err, errs.CodeAPNSUnavailable)
	if _, err := c.Send(t.Context(), push(apns.Sandbox)); !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 3 {
		t.Fatalf("err = %v after %d requests, want the caller's threshold of one failure to open it", err, u.count())
	}
}

func TestWithBreaker_keepsTheDefaultsForWhatItLeavesUnset(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var names []string
	u := &upstream{replies: []reply{rejected(http.StatusInternalServerError, "InternalServerError")}}
	c := newClient(t, u, apns.WithBreaker(gobreaker.Settings{
		OnStateChange: func(name string, _, _ gobreaker.State) {
			mu.Lock()
			defer mu.Unlock()
			names = append(names, name)
		},
	}))
	for range 4 {
		mustSend(t, c, push(apns.Sandbox))
	}
	mu.Lock()
	quiet := len(names) == 0
	mu.Unlock()
	if !quiet {
		t.Fatalf("breaker %v changed state after four failures, want five to be needed", names)
	}

	mustSend(t, c, push(apns.Sandbox))

	_, err := c.Send(t.Context(), push(apns.Sandbox))
	mu.Lock()
	defer mu.Unlock()
	if !errors.Is(err, gobreaker.ErrOpenState) || !slices.Equal(names, []string{"apns-sandbox"}) {
		t.Fatalf("err = %v, state changes %v, want the fifth failure to open apns-sandbox", err, names)
	}
}

func TestSend_aPanickingSendStillSettlesItsBreakerSlot(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{
		func(*http.Request) (*http.Response, error) { panic("transport bug") },
		accepted(),
	}}
	c := newClient(t, u, apns.WithBreaker(gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 1 },
	}))
	func() {
		defer func() { _ = recover() }()
		_, _ = c.Send(t.Context(), push(apns.Sandbox))
	}()

	_, err := c.Send(t.Context(), push(apns.Sandbox))

	if !errors.Is(err, gobreaker.ErrOpenState) || u.count() != 1 {
		t.Fatalf("err = %v after %d requests, want a panic to open the breaker", err, u.count())
	}
}
