package apns_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestSend_theDeadlineComesFromConfigUnlessWithTimeoutSaysOtherwise(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []apns.Option
		want time.Duration
	}{
		{"configured 10s", nil, 10 * time.Second},
		{"option", []apns.Option{apns.WithTimeout(2 * time.Second)}, 2 * time.Second},
		{
			"longer than the 60s apns2 sets on its own client",
			[]apns.Option{apns.WithTimeout(90 * time.Second)},
			90 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				c := newClient(t, &upstream{replies: []reply{hang}}, tt.opts...)
				start := now()

				_, err := c.Send(t.Context(), push(apns.Sandbox))

				wantCode(t, err, errs.CodeAPNSUnavailable)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("err = %v, want the deadline in the chain", err)
				}
				if got := now().Sub(start); got != tt.want {
					t.Fatalf("Send returned after %s, want %s", got, tt.want)
				}
			})
		})
	}
}

func now() time.Time { return clock.Real{}.Now() }
