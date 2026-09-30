package domain

import "github.com/monaco/monaco/apps/backend/internal/platform/money"

func Eligible(netContributed money.SignedMicros, minimum money.Micros) bool {
	net, err := netContributed.Micros()
	return err == nil && !net.IsZero() && net.Cmp(minimum) >= 0
}
