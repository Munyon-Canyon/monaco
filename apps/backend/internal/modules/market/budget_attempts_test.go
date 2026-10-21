package market_test

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type scriptedTransport struct {
	mu       sync.Mutex
	statuses []int
	at       []time.Time
}

func (s *scriptedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statuses[min(len(s.at), len(s.statuses)-1)]
	s.at = append(s.at, clock.Real{}.Now())
	return &http.Response{
		StatusCode: status, Header: http.Header{}, Request: r,
		Body: io.NopCloser(strings.NewReader(`{"prices":[[1700000000000,1.5]],"total_volumes":[]}`)),
	}, nil
}

func (s *scriptedTransport) sent() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.at...)
}

func (r *budgetRig) clientOver(rt http.RoundTripper) *coingecko.Client {
	cfg := config.Config{
		CoinGecko: config.CoinGecko{BaseURL: "http://coingecko.test"},
		Timeouts:  config.Timeouts{CoinGecko: 30 * time.Second},
	}
	return coingecko.New(
		httpclient.New("coingecko", append(coingecko.Options(cfg), httpclient.WithTransport(rt))...), "key",
	).WithBudget(r.budget)
}

func TestCoinGeckoClient_aRetryIsLimitedAndCountedLikeTheFirstAttempt(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	rt := &scriptedTransport{statuses: []int{http.StatusServiceUnavailable, http.StatusOK}}
	if _, err := r.clientOver(rt).MarketChart(r.ctx(t), marketfake.AAPLx().Mint, 1); err != nil {
		t.Fatal(err)
	}
	sent := rt.sent()
	if len(sent) != 2 || r.usage(t).Calls != 2 {
		t.Fatalf("sent %d requests and counted %d, want 2 of each", len(sent), r.usage(t).Calls)
	}
	if gap := sent[1].Sub(sent[0]); gap < time.Minute/25-50*time.Millisecond {
		t.Fatalf("the retry came %s after the first attempt, want the limiter's %s spacing", gap, time.Minute/25)
	}
}

func TestCoinGeckoClient_aRetryStopsWhenTheHardCapIsReachedBetweenAttempts(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.setCalls(t, 94)
	rt := &scriptedTransport{statuses: []int{http.StatusServiceUnavailable, http.StatusOK}}
	_, err := r.clientOver(rt).MarketChart(r.ctx(t), marketfake.AAPLx().Mint, 1)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(rt.sent()) != 1 || r.usage(t).Calls != 95 {
		t.Fatalf("err = %v after %d requests with %d counted, want unavailable after 1 and 95",
			err, len(rt.sent()), r.usage(t).Calls)
	}
}

func TestCoinGeckoClient_aCountThatCannotBeWrittenSendsNothing(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.exec(t, `ALTER TABLE coingecko_usage RENAME TO coingecko_usage_gone`)
	rt := &scriptedTransport{statuses: []int{http.StatusOK}}
	_, err := r.clientOver(rt).MarketChart(r.ctx(t), marketfake.AAPLx().Mint, 1)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(rt.sent()) != 0 {
		t.Fatalf("err = %v after %d requests, want unavailable and none sent", err, len(rt.sent()))
	}
}
