package ranking_test

import (
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

func TestModule_hasNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := ranking.New(module.Deps{})
	if m.Name() != "ranking" || testkit.Serves(m.Mount, "GET", "/v1/assets") || len(m.Consumers()) != 0 ||
		m.Pollers() != nil {
		t.Fatalf("module = %s, consumers %v, pollers %v", m.Name(), m.Consumers(), m.Pollers())
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
