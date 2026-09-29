package testkit

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	eventuallyTick = 10 * time.Millisecond
	settleWithin   = 10 * time.Second
)

func Eventually(t *testing.T, cond func() bool, within time.Duration) {
	t.Helper()
	deadline := time.NewTimer(within)
	defer deadline.Stop()
	tick := time.NewTicker(eventuallyTick)
	defer tick.Stop()
	for !cond() {
		select {
		case <-deadline.C:
			t.Fatalf("testkit.Eventually: condition still false after %s", within)
		case <-tick.C:
		}
	}
}

func AssertNoRedelivery(t *testing.T, consumer jetstream.Consumer) *jetstream.ConsumerInfo {
	t.Helper()
	var info *jetstream.ConsumerInfo
	Eventually(t, func() bool {
		var err error
		info, err = consumer.Info(t.Context())
		if err != nil {
			t.Fatalf("testkit.AssertNoRedelivery: %v", err)
		}
		return info.NumAckPending == 0
	}, settleWithin)
	if info.NumRedelivered != 0 {
		t.Fatalf("testkit.AssertNoRedelivery: %d messages redelivered with no ack pending", info.NumRedelivered)
	}
	return info
}
