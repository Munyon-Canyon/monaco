package apns_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result apns.Result
		want   apns.Outcome
	}{
		{"accepted", apns.Result{Status: 200}, apns.Delivered},
		{"unregistered", apns.Result{Status: 410, Reason: "Unregistered"}, apns.TokenDead},
		{"bad device token", apns.Result{Status: 400, Reason: "BadDeviceToken"}, apns.TokenDead},
		{"too many requests", apns.Result{Status: 429, Reason: "TooManyRequests"}, apns.Retry},
		{"lowest server error", apns.Result{Status: 500}, apns.Retry},
		{"highest server error", apns.Result{Status: 599}, apns.Retry},
		{"forbidden", apns.Result{Status: 403, Reason: "InvalidProviderToken"}, apns.AuthFailed},
		{"expired provider token", apns.Result{Status: 403, Reason: "ExpiredProviderToken"}, apns.AuthFailed},
		{"bad topic", apns.Result{Status: 400, Reason: "BadTopic"}, apns.Rejected},
		{"payload too large", apns.Result{Status: 413, Reason: "PayloadTooLarge"}, apns.Rejected},
		{"gone as an expired token", apns.Result{Status: 410, Reason: "ExpiredToken"}, apns.TokenDead},
		{"gone without a reason", apns.Result{Status: 410}, apns.TokenDead},
		{"bad request without a reason", apns.Result{Status: 400}, apns.Rejected},
		{"just below the server errors", apns.Result{Status: 499}, apns.Rejected},
		{"past the server errors", apns.Result{Status: 600}, apns.Rejected},
		{"no answer at all", apns.Result{}, apns.Rejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := apns.Classify(tt.result); got != tt.want {
				t.Fatalf("Classify(%+v) = %d, want %d", tt.result, got, tt.want)
			}
		})
	}
}
