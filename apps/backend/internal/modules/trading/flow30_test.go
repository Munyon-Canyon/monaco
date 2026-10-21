package trading_test

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"testing"
	"time"

	cabalmod "github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabal "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type flowCabals struct{}

func (flowCabals) Status(context.Context, ids.CabalID) (cabal.Status, error) {
	return cabal.StatusActive, nil
}

func (flowCabals) SlippageBps(context.Context, ids.CabalID) (int32, error) { return 100, nil }

func (flowCabals) TreasuryWallet(_ context.Context, id ids.CabalID) (cabal.TreasuryWallet, error) {
	return cabal.TreasuryWallet{
		CabalID: id, PrivyWalletID: treasuryWallet, Address: chainfake.WalletAddress(treasuryWallet),
	}, nil
}

func (flowCabals) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) { return true, nil }

type flowChain struct {
	mu     sync.Mutex
	hashes map[string]string
}

func (*flowChain) TokenBalance(context.Context, platform.SolanaAddress, platform.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(1_000_000_000, 6), nil
}

func (*flowChain) MintConfig(context.Context, platform.SolanaAddress) (solana.MintConfig, error) {
	return solana.MintConfig{}, nil
}

func (*flowChain) Retryable(context.Context, ids.ProposalID) (bool, error) { return true, nil }

func (*flowChain) Listing(context.Context, multichain.ListingID) (multichain.Listing, error) {
	return flows.RoutedListing(), nil
}

func (*flowChain) Best(
	_ context.Context, _ multichain.StockID, side multichain.Side, amount money.Micros,
) (multichain.Comparison, error) {
	l := flows.RoutedListing()
	return multichain.Comparison{
		StockID: l.StockID, Side: side, Winner: l.ID,
		Ranked: []multichain.Quote{{
			ListingID: l.ID, Network: l.Network, Venue: l.Venue, Side: side, InputMicros: amount, NetMicros: amount,
			Costs: []multichain.Cost{{
				Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(flows.RoutedBridgeFee),
			}},
		}},
	}, nil
}

func (*flowChain) ListingByAsset(context.Context, string) (multichain.Listing, error) {
	return flows.RoutedListing(), nil
}

func (*flowChain) TopUpSOL(context.Context, platform.SolanaAddress) error { return nil }

func (*flowChain) EnsureTreasuryEVMWallet(context.Context, ids.CabalID) (multichain.EVMWallet, error) {
	return flows.RoutedTreasuryEVM(), nil
}

func (*flowChain) Quote(
	_ context.Context,
	l multichain.Listing,
	side multichain.Side,
	in money.Micros,
) (multichain.Quote, error) {
	return multichain.Quote{
		ListingID: l.ID, Side: side, InputMicros: in, GrossOutput: big.NewInt(flows.RoutedRequotedUnits),
	}, nil
}

func (c *flowChain) Transfer(
	_ context.Context,
	_ string,
	_ multichain.TransferSpec,
	key string,
) (multichain.ActionID, error) {
	return multichain.ActionID("privy-action-" + key), nil
}

func (*flowChain) Action(_ context.Context, _ string, id multichain.ActionID) (multichain.ActionStatus, error) {
	return multichain.ActionStatus{ID: id, State: multichain.ActionSucceeded, Steps: []multichain.ActionStep{
		{Network: platform.NetworkSolana, TxHash: "arrival:" + string(id), State: multichain.StepConfirmed},
	}}, nil
}

func (*flowChain) InboundAmount(
	context.Context, platform.Signature, platform.SolanaAddress, platform.Mint,
) (money.BaseUnits, error) {
	return money.NewBaseUnits(flows.RoutedSaleProceeds-flows.RoutedBridgeFee, 6), nil
}

func (*flowChain) Inbound(
	_ context.Context, _ platform.Network, _, _ platform.EVMAddress, _ time.Time, atMost money.Micros, exclude []string,
) (app.EVMArrival, bool, error) {
	landed := money.MicrosFromUint64(atMost.Uint64() - flows.RoutedBridgeFee)
	return app.EVMArrival{Amount: landed, TxHash: fmt.Sprintf("0x%064x", len(exclude)+1)}, true, nil
}

func (c *flowChain) Send(_ context.Context, _ string, _ multichain.EVMTransaction, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.hashes[key]; !ok {
		c.hashes[key] = "0xf30" + strconv.Itoa(len(c.hashes)+1)
	}
	return c.hashes[key], nil
}

func (*flowChain) Build(_ context.Context, spec app.EVMSwapSpec) (app.EVMSwapPlan, error) {
	plan := app.EVMSwapPlan{Swap: multichain.EVMTransaction{Network: spec.Network, To: spec.TokenOut}}
	if spec.TokenOut.String() == flows.RoutedEVMToken {
		plan.MinOut = 303_000_000
		return plan, nil
	}
	plan.OutAmount, plan.MinOut = flows.RoutedSaleProceeds, flows.RoutedSaleProceeds
	return plan, nil
}

func (*flowChain) Choose(context.Context, app.EVMSwapSpec, []app.EVMSwapCandidate) (app.EVMSwapPlan, error) {
	return app.EVMSwapPlan{}, nil
}

func (*flowChain) Receipt(context.Context, platform.Network, string) (app.EVMFillState, error) {
	return app.FillSucceeded, nil
}

func (*flowChain) Fill(
	_ context.Context,
	_ platform.Network,
	_ string,
	out, _ platform.EVMAddress,
) (app.EVMFill, error) {
	if out.String() == flows.RoutedEVMToken {
		return app.EVMFill{State: app.FillSucceeded, Out: flows.RoutedFilledUnits}, nil
	}
	return app.EVMFill{State: app.FillSucceeded, Out: flows.RoutedSaleProceeds}, nil
}

func flow30Modules() []func(module.Deps) module.Module {
	return []func(module.Deps) module.Module{
		func(d module.Deps) module.Module {
			fake := &flowChain{hashes: map[string]string{}}
			d.Config.Solana.USDCMint, d.Config.Solana.RPCURL = string(testkit.USDCMint), "http://fakes/rpc/"
			d.Config.Timeouts.RPC = time.Second
			d.Config.Trade.SwapSweepInterval, d.Config.Trade.SwapSweepAge = time.Minute, app.SwapSweepAge
			layer := newLayerEnvOn(d.Pool)
			return trading.New(d,
				trading.WithEnginePorts(app.EnginePorts{
					Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), Cabals: flowCabals{},
					Pauses: fakes.NewPauses(), Proposals: fake, Balances: fake,
				}),
				trading.WithChain(layer.venue, layer.signer), trading.WithChainReader(layer.fills),
				trading.WithRoutePorts(app.RoutePorts{
					Listings:    fake,
					TreasuryEVM: fake,
					Quoter:      fake,
					Actions:     fake,
					Sender:      fake,
					Venue:       fake,
					Router:      fake,
					TopUp:       fake,
					Arrivals:    fake,
					EVMArrivals: fake,
					EVM:         true,
				}),
			)
		},
		func(d module.Deps) module.Module {
			d.Config.Solana.USDCMint = string(testkit.USDCMint)
			return treasury.New(d)
		},
	}
}

func runFlow30Step(t *testing.T, name string, modules ...func(module.Deps) module.Module) {
	t.Helper()
	s := scenario.New(t, scenario.WithModules(append(flow30Modules(), modules...)...))
	for _, step := range flows.Steps() {
		if step.Flow == "30" && step.Name == name {
			step.Script(s)
			return
		}
	}
	t.Fatalf("flow 30 has no %q step", name)
}

func TestFlowStep30_RoutedBuyOnEVM(t *testing.T) {
	t.Parallel()
	runFlow30Step(t, "Routed buy on EVM")
}

func TestFlowStep30_RoutedSell(t *testing.T) {
	t.Parallel()
	runFlow30Step(t, "Routed sell")
}

type aaplListings struct{}

func (aaplListings) ListingByAsset(context.Context, string) (multichain.Listing, error) {
	l := flows.RoutedListing()
	l.StockID, l.Enabled = aaplStock(), true
	return l, nil
}

func aaplStock() multichain.StockID {
	var stock multichain.StockID
	_ = stock.UnmarshalText([]byte(marketfake.AAPLx().ID.String()))
	return stock
}

type unitQuoter struct{}

func (unitQuoter) Quote(
	_ context.Context, l multichain.Listing, _ multichain.Side, micros money.Micros,
) (multichain.Quote, error) {
	return multichain.Quote{ListingID: l.ID, GrossOutput: new(big.Int).SetUint64(micros.Uint64() * 2)}, nil
}

func TestFlowStep30_GovernedRoutedSell(t *testing.T) {
	t.Parallel()
	runFlow30Step(t, "Governed routed sell", func(d module.Deps) module.Module {
		d.Config.Solana.USDCMint = string(testkit.USDCMint)
		return governance.New(d, governance.WithPorts(governance.Ports{
			Cabals: cabalmod.New(d).Queries(), Assets: marketfake.NewCatalog(marketfake.Fixtures()...),
			Treasury: treasury.New(d).Queries(), Listings: aaplListings{}, Quoter: unitQuoter{},
			Balances: &flowChain{hashes: map[string]string{}},
		}))
	})
}
