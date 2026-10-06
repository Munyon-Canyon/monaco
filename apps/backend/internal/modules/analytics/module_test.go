package analytics_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_isNamedAnalyticsAndHasNoRoutesPollersOrConsumerWhileNothingIsExported(t *testing.T) {
	t.Parallel()
	m := analytics.NewWithExports(module.Deps{}, analytics.NewRegistry())
	if m.Name() != "analytics" || testkit.Serves(m.Mount, "GET", "/v1/assets") || m.Consumers() != nil ||
		m.Pollers() != nil {
		t.Fatalf("module %s serves routes, consumers %v, pollers %v; want none", m.Name(), m.Consumers(),
			m.Pollers())
	}
}

func TestModule_exportsEachProductSubjectOnTheAnalyticsDurable(t *testing.T) {
	t.Parallel()
	m := analytics.New(module.Deps{})
	consumers := m.Consumers()
	if len(consumers) != 1 || consumers[0].Durable != "analytics" || testkit.Serves(m.Mount, "GET", "/v1/assets") ||
		m.Pollers() != nil {
		t.Fatalf("consumers %+v, want one analytics durable and no routes or pollers", consumers)
	}
	got := make([]string, 0, len(consumers[0].Handlers))
	for _, h := range consumers[0].Handlers {
		got = append(got, h.Name)
	}
	want := []string{
		"analytics.posthog.proposal.passed",
		"analytics.posthog.proposal.failed",
		"analytics.posthog.proposal.expired",
		"analytics.posthog.trade.confirmed",
		"analytics.posthog.trade.blocked",
		"analytics.posthog.trade.failed",
		"analytics.posthog.deposit.credited",
		"analytics.posthog.cabal.funded",
		"analytics.posthog.cashout.completed",
		"analytics.posthog.cashout.partial",
		"analytics.posthog.cashout.failed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("handlers = %v, want %v", got, want)
	}
}
