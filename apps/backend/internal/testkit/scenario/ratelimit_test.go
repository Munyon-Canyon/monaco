package scenario

import (
	"context"
	"net/http"
	"sync"
	"testing"
)

func TestUntilRateLimited_stopsAtTheFirstRefusal(t *testing.T) {
	t.Parallel()
	s := &Scenario{t: &recordingT{T: t, ctx: context.Background}}
	answers := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusOK}
	sent := 0
	UntilRateLimited(10, func(s *Scenario) {
		s.last = &response{status: answers[sent]}
		sent++
	})(s)
	if sent != 3 {
		t.Fatalf("sent %d requests, want 3", sent)
	}
}

func TestUntilRateLimited_failsWhenNothingIsRefused(t *testing.T) {
	t.Parallel()
	r := &recordingT{T: t, ctx: context.Background}
	var wg sync.WaitGroup
	wg.Go(func() {
		UntilRateLimited(3, func(s *Scenario) { s.last = &response{status: http.StatusOK} })(&Scenario{t: r})
	})
	wg.Wait()
	if r.failed == "" {
		t.Fatal("no failure after 3 requests that were never refused")
	}
}
