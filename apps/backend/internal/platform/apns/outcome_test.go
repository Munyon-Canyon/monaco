package apns_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func TestSend_mapsEachAPNsAnswerToItsOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		reply reply
		want  apns.Outcome
		res   apns.Result
	}{
		{
			"accepted", accepted(), apns.Delivered,
			apns.Result{Status: 200, APNsID: apnsID},
		},
		{
			"unregistered", respond(http.StatusGone, `{"reason":"Unregistered","timestamp":1690000000000}`),
			apns.TokenDead,
			apns.Result{Status: 410, Reason: "Unregistered"},
		},
		{
			"bad device token", rejected(http.StatusBadRequest, "BadDeviceToken"),
			apns.TokenDead,
			apns.Result{Status: 400, Reason: "BadDeviceToken"},
		},
		{
			"too many requests",
			respond(http.StatusTooManyRequests, `{"reason":"TooManyRequests"}`, "Retry-After", "3"),
			apns.Retry,
			apns.Result{Status: 429, Reason: "TooManyRequests", RetryAfter: 3 * time.Second},
		},
		{
			"internal error", rejected(http.StatusInternalServerError, "InternalServerError"),
			apns.Retry,
			apns.Result{Status: 500, Reason: "InternalServerError"},
		},
		{
			"forbidden", rejected(http.StatusForbidden, "InvalidProviderToken"),
			apns.AuthFailed,
			apns.Result{Status: 403, Reason: "InvalidProviderToken"},
		},
		{
			"bad topic", rejected(http.StatusBadRequest, "BadTopic"),
			apns.Rejected,
			apns.Result{Status: 400, Reason: "BadTopic"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := mustSend(t, newClient(t, &upstream{replies: []reply{tt.reply}}), push(apns.Sandbox))
			if res != tt.res {
				t.Fatalf("Send = %+v, want %+v", res, tt.res)
			}
			if got := apns.Classify(res); got != tt.want {
				t.Fatalf("Classify(%+v) = %d, want %d", res, got, tt.want)
			}
		})
	}
}
