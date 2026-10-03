package trading_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	engineHandler = "trading.engine"
	buyMicros     = 25_000_000
	quotedOut     = 21_000_000
)

type balancesFake struct {
	testkit.Faults
	ledger *chainfake.Ledger
	mu     sync.Mutex
	mints  map[platform.SolanaAddress]solana.MintConfig
}

func (b *balancesFake) TokenBalance(
	ctx context.Context, owner platform.SolanaAddress, mint platform.Mint,
) (money.BaseUnits, error) {
	if err := b.Check("TokenBalance"); err != nil {
		return money.BaseUnits{}, err
	}
	return b.ledger.TokenBalance(ctx, owner, mint)
}

func (b *balancesFake) MintConfig(_ context.Context, mint platform.SolanaAddress) (solana.MintConfig, error) {
	if err := b.Check("MintConfig"); err != nil {
		return solana.MintConfig{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mints[mint], nil
}

func (b *balancesFake) setFee(mint platform.SolanaAddress, bps uint16, maxFee uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mints[mint] = solana.MintConfig{TransferFeeBps: bps, MaxFee: money.NewBaseUnits(maxFee, 8)}
}

type proposalsFake struct {
	testkit.Faults
	mu     sync.Mutex
	status governance.Status
}

func (p *proposalsFake) Status(context.Context, ids.ProposalID) (governance.Status, error) {
	if err := p.Check("Status"); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status, nil
}

func (p *proposalsFake) set(status governance.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

type engineEnv struct {
	*layerEnv
	cabal     ids.CabalID
	wallet    cabal.TreasuryWallet
	catalog   *marketfake.CatalogFake
	cabals    *fakes.Cabal
	pauses    *fakes.Pauses
	proposals *proposalsFake
	balances  *balancesFake
	ledger    *chainfake.Ledger
}

func newEngineEnv(t *testing.T) *engineEnv {
	t.Helper()
	return newEngineEnvOn(newLayerEnv(t))
}

func newEngineEnvOn(layer *layerEnv) *engineEnv {
	e := &engineEnv{layerEnv: layer}
	e.cabal = ids.CabalIDFrom(e.ids.NewV7())
	e.wallet = cabal.TreasuryWallet{
		CabalID: e.cabal, PrivyWalletID: treasuryWallet, Address: chainfake.WalletAddress(treasuryWallet),
	}
	e.catalog = marketfake.NewCatalog(marketfake.Fixtures()...)
	e.seedCabal(cabal.StatusActive, 100)
	e.pauses = fakes.NewPauses()
	e.proposals = &proposalsFake{status: governance.StatusPassed}
	e.ledger = chainfake.NewLedger(e.clk)
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 100_000_000)
	e.balances = &balancesFake{ledger: e.ledger, mints: map[platform.SolanaAddress]solana.MintConfig{}}
	e.quote(usdcToken(), aaplxToken(), quotedOut, true)
	return e
}

func (e *engineEnv) seedCabal(status cabal.Status, slippageBps int32) {
	e.cabals = fakes.NewCabal([]fakes.CabalSeed{{
		View: cabal.View{ID: e.cabal, Status: status}, Rules: cabal.Rules{SlippageBps: slippageBps}, Wallet: e.wallet,
	}}, nil)
}

func (e *engineEnv) quote(in, out platform.Mint, outAmount uint64, routable bool) {
	e.jup.SetQuote(jupiterMint(in), jupiterMint(out), jupiter.Quote{
		InAmount: money.NewBaseUnits(0, in.Decimals), OutAmount: money.NewBaseUnits(outAmount, out.Decimals),
		Routable: routable,
	})
}

type liveCabals struct{ e *engineEnv }

func (c liveCabals) Status(ctx context.Context, id ids.CabalID) (cabal.Status, error) {
	return c.e.cabals.Status(ctx, id)
}

func (c liveCabals) SlippageBps(ctx context.Context, id ids.CabalID) (int32, error) {
	return c.e.cabals.SlippageBps(ctx, id)
}

func (c liveCabals) TreasuryWallet(ctx context.Context, id ids.CabalID) (cabal.TreasuryWallet, error) {
	return c.e.cabals.TreasuryWallet(ctx, id)
}

func (e *engineEnv) ports() app.EnginePorts {
	return app.EnginePorts{
		Catalog: e.catalog, Cabals: liveCabals{e: e}, Pauses: e.pauses, Proposals: e.proposals, Balances: e.balances,
	}
}

func (e *engineEnv) handler() *app.ExecuteTradeHandler {
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), UoW: e.uow, Reads: e.reads, Venue: e.venue, Ports: e.ports(), USDC: usdcToken(),
	})
}

func (e *engineEnv) buy() app.ExecuteTrade {
	return app.ExecuteTrade{
		ProposalID: ids.ProposalIDFrom(e.ids.NewV7()), CabalID: e.cabal, Action: domain.ActionBuy, Symbol: "AAPLx",
		Mint: aaplxMint, USDCMicros: money.MicrosFromUint64(buyMicros), QuoteOutAmount: quotedOut,
	}
}

func (e *engineEnv) sell(units uint64) app.ExecuteTrade {
	cmd := e.buy()
	cmd.Action, cmd.USDCMicros, cmd.TokenAmount = domain.ActionSell, money.Micros{}, units
	return cmd
}

func (e *engineEnv) delivery(t *testing.T, cmd app.ExecuteTrade) bus.Delivery {
	t.Helper()
	ev := busevents.ProposalPassed{
		V: 1, ProposalID: cmd.ProposalID.UUID(), CabalID: cmd.CabalID.UUID(), Kind: string(cmd.Action),
		Symbol: cmd.Symbol, Mint: cmd.Mint, USDCMicros: cmd.USDCMicros, TokenAmount: cmd.TokenAmount,
		QuoteOutAmount: cmd.QuoteOutAmount, ProposerID: e.ids.NewV7(),
	}
	var id uuid.UUID
	err := e.uow.Do(actorContext(t.Context()), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	})
	if err == nil {
		err = e.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE type = 'proposal.passed'
			AND aggregate_id = $1 ORDER BY id DESC LIMIT 1`, ev.ProposalID).Scan(&id)
	}
	if err != nil {
		t.Fatal(err)
	}
	return bus.Delivery{Handler: engineHandler, EventID: ids.EventIDFrom(id), At: e.clk.Now()}
}

func (e *engineEnv) handle(t *testing.T, d bus.Delivery, cmd app.ExecuteTrade) error {
	t.Helper()
	return e.handler().Handle(actorContext(t.Context()), d, cmd, nil)
}

func (e *engineEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *engineEnv) recorded(t *testing.T, d bus.Delivery) bool {
	t.Helper()
	return e.count(t, `SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id = $2`,
		d.Handler, d.EventID.UUID()) == 1
}

func (e *engineEnv) blocked(t *testing.T, proposal ids.ProposalID) []map[string]any {
	t.Helper()
	rows, err := e.pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = 'trade.blocked' AND aggregate_id = $1 ORDER BY id`, proposal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var p map[string]any
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *engineEnv) swapsOf(t *testing.T, proposal ids.ProposalID) []uuid.UUID {
	t.Helper()
	return e.swapIDs(t, proposal.UUID())
}
