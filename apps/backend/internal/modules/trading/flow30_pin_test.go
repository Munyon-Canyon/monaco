package trading_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlowStep30_PinnedBuyBlockedWhileItsChainTradingIsOff(t *testing.T) {
	t.Parallel()
	flows.F30PinnedBuyChainTradingOff(scenario.New(t, scenario.WithModules(
		func(d module.Deps) module.Module { return governance.New(d) },
		func(d module.Deps) module.Module {
			fake := &flowChain{hashes: map[string]string{}}
			d.Config.Solana.USDCMint, d.Config.Solana.RPCURL = string(testkit.USDCMint), "http://fakes/rpc/"
			d.Config.Timeouts.RPC = time.Second
			layer := newLayerEnvOn(d.Pool)
			return trading.New(d,
				trading.WithEnginePorts(app.EnginePorts{
					Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), Cabals: flowCabals{},
					Pauses: fakes.NewPauses(), Proposals: fake, Balances: fake,
				}),
				trading.WithChain(layer.venue, layer.signer), trading.WithChainReader(layer.fills),
				trading.WithRoutePorts(app.RoutePorts{
					Listings: fake, TreasuryEVM: fake, Quoter: fake, Actions: fake, Sender: fake, Venue: fake,
					Router: fake, TopUp: fake, Arrivals: fake, EVMArrivals: fake,
				}),
			)
		},
	)))
}
