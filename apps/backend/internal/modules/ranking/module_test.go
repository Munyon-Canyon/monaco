package ranking_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_hasNoRoutesOrPollersYet(t *testing.T) {
	t.Parallel()
	m := ranking.New(module.Deps{})
	if m.Name() != "ranking" || testkit.Serves(m.Mount, "GET", "/v1/assets") || m.Pollers() != nil {
		t.Fatalf("module = %s, pollers %v", m.Name(), m.Pollers())
	}
}

func TestModule_declaresEachConsumerWithItsOwnDurable(t *testing.T) {
	t.Parallel()
	consumers := ranking.New(module.Deps{}).Consumers()
	got := make([]string, 0, len(consumers))
	for _, c := range consumers {
		got = append(got, c.Durable)
	}
	if !slices.Contains(got, "ranking_membership") {
		t.Fatalf("durables = %q, want ranking_membership", got)
	}
}

func TestModule_wiresEveryValuationReadPort(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	rankingModule := ranking.New(d)
	module.NewSet(
		rankingModule,
		market.New(d),
		treasury.New(d),
		funding.New(d),
		cabal.New(d),
		identity.New(d),
	)
	ports := rankingModule.Ports()
	if ports.Treasury == nil || ports.Funding == nil || ports.Cabals == nil || ports.Users == nil {
		t.Fatalf("ports = %+v, want every query port wired", ports)
	}
}

func TestModule_usesPortsOption(t *testing.T) {
	t.Parallel()
	ports := app.Ports{}
	if got := ranking.New(module.Deps{}, ranking.WithPorts(ports)).Ports(); got != ports {
		t.Fatalf("Ports = %+v, want %+v", got, ports)
	}
}
