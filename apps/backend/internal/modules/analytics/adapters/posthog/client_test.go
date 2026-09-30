package posthog_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/posthog"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/posthogfake"
)

const apiKey = "ph-test-key"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newClient(t *testing.T, host string, opts ...httpclient.Option) posthog.Client {
	t.Helper()
	cfg := config.Config{
		PostHog:  config.PostHog{APIKey: apiKey, Host: host},
		Timeouts: config.Timeouts{PostHog: time.Minute},
	}
	return posthog.New(cfg, func(name string, base ...httpclient.Option) *httpclient.Client {
		if name != "posthog" {
			t.Errorf("client named %q, want posthog", name)
		}
		return httpclient.New(name, append(base, opts...)...)
	})
}

func sample() app.Capture {
	clk := testkit.NewClock(clock.Real{}.Now().Truncate(time.Second))
	return app.Capture{
		UUID:       testkit.NewIDs(1).NewV7(),
		Event:      "probe_fired",
		DistinctID: "user-1",
		Timestamp:  clk.Now(),
		Properties: map[string]any{"cabal": "alpha", "first": true},
		Set:        map[string]any{"tier": "gold"},
	}
}

func sameCapture(t *testing.T, got, want fakes.PostHogCapture) {
	t.Helper()
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("timestamp = %s, want %s", got.Timestamp, want.Timestamp)
	}
	got.Timestamp, want.Timestamp = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("capture = %+v, want %+v", got, want)
	}
}

func TestClient_Capture_sendsEachCaptureTheWayPostHogExpectsIt(t *testing.T) {
	t.Parallel()
	fake := posthogfake.New(t)
	client := newClient(t, fake.Host())
	first, second := sample(), sample()
	second.UUID, second.Set, second.Properties = testkit.NewIDs(2).NewV7(), nil, nil
	if err := client.Capture(t.Context(), []app.Capture{first, second}); err != nil {
		t.Fatal(err)
	}
	got := fake.Captures()
	if len(got) != 2 {
		t.Fatalf("the fake holds %d captures, want 2: %+v", len(got), got)
	}
	sameCapture(t, got[0], fakes.PostHogCapture{
		APIKey: apiKey, UUID: first.UUID, Event: first.Event, DistinctID: first.DistinctID,
		Timestamp: first.Timestamp, Properties: first.Properties, Set: first.Set,
	})
	sameCapture(t, got[1], fakes.PostHogCapture{
		APIKey: apiKey, UUID: second.UUID, Event: second.Event, DistinctID: second.DistinctID,
		Timestamp: second.Timestamp, Properties: map[string]any{},
	})
}

func TestClient_Capture_sendsTheTimestampInUTCWithoutTouchingTheCallersProperties(t *testing.T) {
	t.Parallel()
	fake := posthogfake.New(t)
	client := newClient(t, fake.Host())
	c := sample()
	c.Timestamp = c.Timestamp.In(time.FixedZone("offset", 5*60*60))
	if err := client.Capture(t.Context(), []app.Capture{c}); err != nil {
		t.Fatal(err)
	}
	got := fake.Captures()
	if len(got) != 1 || got[0].Timestamp.Location() != time.UTC || !got[0].Timestamp.Equal(c.Timestamp) {
		t.Fatalf("captures = %+v, want one stamped at %s in UTC", got, c.Timestamp)
	}
	if _, leaked := c.Properties["$set"]; leaked || len(c.Properties) != 2 {
		t.Fatalf("caller's properties = %v, want them left alone", c.Properties)
	}
}

func TestClient_Capture_mapsStatusesToTheirCodes(t *testing.T) {
	t.Parallel()
	tests := map[int]errs.Code{
		http.StatusTooManyRequests:       errs.CodePostHogUnavailable,
		http.StatusInternalServerError:   errs.CodePostHogUnavailable,
		http.StatusBadGateway:            errs.CodePostHogUnavailable,
		http.StatusServiceUnavailable:    errs.CodePostHogUnavailable,
		http.StatusGatewayTimeout:        errs.CodePostHogUnavailable,
		http.StatusBadRequest:            errs.CodePostHogRejected,
		http.StatusUnauthorized:          errs.CodePostHogRejected,
		http.StatusForbidden:             errs.CodePostHogRejected,
		http.StatusNotFound:              errs.CodePostHogRejected,
		http.StatusRequestEntityTooLarge: errs.CodePostHogRejected,
	}
	for status, want := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			fake := posthogfake.New(t)
			fake.Fail(t, status, 1)
			err := newClient(t, fake.Host()).Capture(t.Context(), []app.Capture{sample()})
			if got := errs.CodeOf(err); got != want || fake.Received() != 0 {
				t.Fatalf("status %d: Capture = %v (%s) with %d received, want %s and none received",
					status, err, got, fake.Received(), want)
			}
		})
	}
}

func TestClient_Capture_recoversOnTheAttemptAfterAFailure(t *testing.T) {
	t.Parallel()
	fake := posthogfake.New(t)
	fake.Fail(t, http.StatusServiceUnavailable, 2)
	client := newClient(t, fake.Host())
	c := sample()
	attempts := make([]error, 0, 3)
	for range 3 {
		attempts = append(attempts, client.Capture(t.Context(), []app.Capture{c}))
	}
	first, second := errs.CodeOf(attempts[0]), errs.CodeOf(attempts[1])
	if first != errs.CodePostHogUnavailable || second != errs.CodePostHogUnavailable || attempts[2] != nil ||
		fake.Received() != 1 {
		t.Fatalf("attempts = %v with %d received, want two post_hog_unavailable, then success and one received",
			attempts, fake.Received())
	}
}

func TestClient_Capture_isUnavailableWhenTheTransportFails(t *testing.T) {
	t.Parallel()
	client := newClient(t, "http://posthog.test", httpclient.WithTransport(
		roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF }),
	))
	err := client.Capture(t.Context(), []app.Capture{sample()})
	if errs.CodeOf(err) != errs.CodePostHogUnavailable || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Capture = %v, want post_hog_unavailable wrapping the transport error", err)
	}
}

func TestClient_Capture_isUnavailableWhenThePerCallDeadlineRunsOut(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		hang := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		client := newClient(t, "http://posthog.test", httpclient.WithTransport(hang))
		err := client.Capture(t.Context(), []app.Capture{sample()})
		if errs.CodeOf(err) != errs.CodePostHogUnavailable || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Capture = %v, want post_hog_unavailable wrapping the deadline", err)
		}
	})
}

func TestClient_Capture_isUnavailableWithoutCallingOutOnceTheBreakerIsOpen(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	failing := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusInternalServerError, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader("")),
		}, nil
	})
	trip := gobreaker.Settings{ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 1 }}
	client := newClient(t, "http://posthog.test", httpclient.WithTransport(failing), httpclient.WithBreaker(trip))
	codes := make([]errs.Code, 0, 2)
	for range 2 {
		codes = append(codes, errs.CodeOf(client.Capture(t.Context(), []app.Capture{sample()})))
	}
	if codes[0] != errs.CodePostHogUnavailable || codes[1] != errs.CodePostHogUnavailable || calls.Load() != 1 {
		t.Fatalf("codes %v after %d calls out, want two post_hog_unavailable and one call", codes, calls.Load())
	}
}

func TestClient_Capture_refusesAPropertyJSONCannotEncode(t *testing.T) {
	t.Parallel()
	fake := posthogfake.New(t)
	c := sample()
	c.Properties = map[string]any{"bad": make(chan int)}
	err := newClient(t, fake.Host()).Capture(t.Context(), []app.Capture{c})
	if errs.CodeOf(err) != errs.CodeInternal || fake.Received() != 0 {
		t.Fatalf("Capture = %v with %d received, want internal and none received", err, fake.Received())
	}
}

func TestClient_Capture_classifiesEveryStatusTheServerCanSend(t *testing.T) {
	t.Parallel()
	tests := map[int]errs.Code{
		http.StatusOK:                  "",
		http.StatusAccepted:            "",
		http.StatusNoContent:           "",
		299:                            "",
		http.StatusMultipleChoices:     errs.CodePostHogRejected,
		399:                            errs.CodePostHogRejected,
		http.StatusBadRequest:          errs.CodePostHogRejected,
		499:                            errs.CodePostHogRejected,
		http.StatusInternalServerError: errs.CodePostHogUnavailable,
		http.StatusNotImplemented:      errs.CodePostHogUnavailable,
		599:                            errs.CodePostHogUnavailable,
	}
	for status, want := range tests {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			replying := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})
			err := newClient(t, "http://posthog.test", httpclient.WithTransport(replying)).
				Capture(t.Context(), []app.Capture{sample()})
			if want == "" && err != nil || want != "" && errs.CodeOf(err) != want {
				t.Fatalf("status %d: Capture = %v, want code %q", status, err, want)
			}
		})
	}
}
