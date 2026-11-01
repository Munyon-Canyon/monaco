package jupiter

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const skippedLogEvery = time.Hour

type skippedMints struct {
	mu     sync.Mutex
	logged map[string]time.Time
}

func newSkippedMints() *skippedMints {
	s := &skippedMints{logged: map[string]time.Time{}}
	s.observe(otel.GetMeterProvider().Meter("github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"))
	return s
}

func (s *skippedMints) observe(meter metric.Meter) {
	_, _ = meter.Int64ObservableGauge("monaco_jupiter_unpriced_mints",
		metric.WithUnit("{mint}"),
		metric.WithDescription("Mints the last Jupiter price poll returned without a usable price."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(s.count()))
			return nil
		}))
}

func (s *skippedMints) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.logged)
}

func (s *skippedMints) due(mint string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.logged[mint]; ok && now.Sub(last) < skippedLogEvery {
		return false
	}
	s.logged[mint] = now
	return true
}

func (s *skippedMints) priced(mint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logged, mint)
}
