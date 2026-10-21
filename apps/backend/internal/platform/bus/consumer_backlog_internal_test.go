package bus

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestConsumerBacklogMerge_takesTheOldestStalledAckFloorAndIgnoresDrainedConsumers(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	floor := func(ago time.Duration) jetstream.SequenceInfo {
		at := now.Add(-ago)
		return jetstream.SequenceInfo{Last: &at}
	}
	drained := &jetstream.ConsumerInfo{AckFloor: floor(time.Hour)}
	slow := &jetstream.ConsumerInfo{AckFloor: floor(90 * time.Second), NumPending: 4, NumAckPending: 1}
	stuck := &jetstream.ConsumerInfo{AckFloor: floor(10 * time.Minute), NumAckPending: 2}
	idle := &jetstream.ConsumerInfo{NumPending: 1}

	var got ConsumerBacklog
	for _, info := range []*jetstream.ConsumerInfo{drained, slow, stuck, idle} {
		got = got.merge(info, now)
	}
	if want := (ConsumerBacklog{Pending: 8, Oldest: 10 * time.Minute}); got != want {
		t.Fatalf("merged backlog = %+v, want %+v", got, want)
	}
}

type failingLister struct{ err error }

func (l failingLister) Info() <-chan *jetstream.ConsumerInfo {
	ch := make(chan *jetstream.ConsumerInfo)
	close(ch)
	return ch
}

func (l failingLister) Err() error { return l.err }

func TestBacklogOf_failsAsUnavailableWhenTheListingBreaks(t *testing.T) {
	t.Parallel()
	got, err := backlogOf(failingLister{err: errors.New("connection closed")}, time.Time{})
	if got != (ConsumerBacklog{}) || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("backlogOf = %+v, %v; want zero and upstream_unavailable", got, err)
	}
}
