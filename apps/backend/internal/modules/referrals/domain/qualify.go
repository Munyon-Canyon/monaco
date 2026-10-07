package domain

import "github.com/monaco/monaco/apps/backend/internal/platform/money"

const MinimumFundingMicros uint64 = 10_000_000

const (
	SkipBelowMinimum    = "below_minimum"
	SkipPhoneUnverified = "phone_unverified"
)

func SkipReason(amount money.Micros, phoneVerified bool) string {
	switch {
	case amount.Cmp(money.MicrosFromUint64(MinimumFundingMicros)) < 0:
		return SkipBelowMinimum
	case !phoneVerified:
		return SkipPhoneUnverified
	}
	return ""
}
