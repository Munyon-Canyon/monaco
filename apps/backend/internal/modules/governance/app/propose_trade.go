package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const usdcDecimals = 6

type Cabals interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
	Rules(ctx context.Context, id ids.CabalID) (cabalport.Rules, error)
	VoterSet(ctx context.Context, id ids.CabalID) ([]ids.UserID, error)
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
	Status(ctx context.Context, id ids.CabalID) (cabalport.Status, error)
}

type Assets interface {
	AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error)
	AssetByMint(ctx context.Context, mint market.Mint) (market.Asset, error)
}

type PinListings interface {
	PinListing(
		ctx context.Context, stock multichain.StockID, n chain.Network,
	) (multichain.ListingID, multichain.Venue, bool, error)
}

type Treasury interface {
	PotValue(ctx context.Context, id ids.CabalID) (money.Micros, error)
	SolanaPositions(ctx context.Context, id ids.CabalID) ([]treasuryport.Position, error)
	CashOutReservations(ctx context.Context) (map[ids.CabalID]money.Micros, error)
}

type Balances interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
}

type TradePorts struct {
	Cabals   Cabals
	Assets   Assets
	Routes   market.Routes
	Router   multichain.Router
	Listings Listings
	Quoter   multichain.Quoter
	Treasury Treasury
	Balances Balances
	USDC     chain.Mint
}

type Transactor interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx db.Tx) error) error
}

type ProposeTrade struct {
	CabalID    ids.CabalID
	ProposerID ids.UserID
	Trade      domain.Trade
}

type OpenedProposal struct {
	Draft           domain.Draft
	Voters          []ids.UserID
	CreatedAt       time.Time
	Needed          int
	ProposerCanVote bool
	USDCMicros      int64
	TokenAmount     int64
	QuoteOutAmount  int64
	PriceComparison *multichain.Comparison
	Stats           Stats
}

type ProposeTradeHandler struct {
	uow   Transactor
	ids   ids.Generator
	clock clock.Clock
	ports TradePorts
	hints Hints
}

func NewProposeTradeHandler(
	uow Transactor, g ids.Generator, c clock.Clock, p TradePorts, hints Hints,
) *ProposeTradeHandler {
	return &ProposeTradeHandler{uow: uow, ids: g, clock: c, ports: p, hints: hints}
}

func (h *ProposeTradeHandler) Handle(ctx context.Context, cmd ProposeTrade) (ids.ProposalID, error) {
	opened, err := h.Open(ctx, cmd)
	if err != nil {
		return ids.ProposalID{}, err
	}
	return opened.Draft.ID, nil
}

func (h *ProposeTradeHandler) proposerVoters(ctx context.Context, cmd ProposeTrade) ([]ids.UserID, error) {
	const op = "governance.ProposeTrade"
	voters, err := h.ports.Cabals.VoterSet(ctx, cmd.CabalID)
	if err != nil {
		return nil, err
	}
	if len(voters) == 0 {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("field", "voters"))
	}
	if !slices.Contains(voters, cmd.ProposerID) {
		return nil, errs.New(errs.CodeNotAVoter, op)
	}
	return voters, nil
}

func (h *ProposeTradeHandler) Open(ctx context.Context, cmd ProposeTrade) (OpenedProposal, error) {
	const op = "governance.ProposeTrade"
	if err := member(ctx, h.ports.Cabals, cmd.CabalID, cmd.ProposerID); err != nil {
		return OpenedProposal{}, err
	}
	if err := notBanned(ctx, h.ports.Cabals, cmd.CabalID, op); err != nil {
		return OpenedProposal{}, err
	}
	rules, err := h.ports.Cabals.Rules(ctx, cmd.CabalID)
	if err != nil {
		return OpenedProposal{}, err
	}
	rule, err := domain.ParseThresholdRule(string(rules.Threshold))
	if err != nil {
		return OpenedProposal{}, err
	}
	voters, err := h.proposerVoters(ctx, cmd)
	if err != nil {
		return OpenedProposal{}, err
	}
	a, err := h.assess(ctx, cmd)
	if err != nil {
		return OpenedProposal{}, err
	}
	now := h.clock.Now()
	p, err := domain.NewProposal(domain.Draft{
		ID: ids.ProposalIDFrom(h.ids.NewV7()), CabalID: cmd.CabalID, ProposerID: cmd.ProposerID, Kind: cmd.Trade.Kind,
		Symbol: a.asset.Symbol, Mint: a.asset.Mint.Address(), USDCMicros: cmd.Trade.USDCMicros,
		TokenAmount: money.NewBaseUnits(cmd.Trade.TokenAmount, a.decimals), Thesis: cmd.Trade.Thesis,
		QuoteOut:  displayQuote(a.asset, a.priced, a.routed, now),
		ExpiresAt: now.Add(rules.ProposalExpiry), Network: a.network,
	})
	if err != nil {
		return OpenedProposal{}, err
	}
	share, err := TreasuryShareBps(shareValue(p.Draft()), a.pot)
	if err != nil {
		return OpenedProposal{}, err
	}
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		params := openParams(p.Draft(), rule, voters, now)
		params.SidesAfterClose = rules.ShowSides
		params.PotValueMicros = a.potColumn()
		return store(ctx, tx, params, a.comparison, created(p.Draft(), len(voters)))
	})
	if err != nil {
		return OpenedProposal{}, err
	}
	h.hints.ProposalCreated(ctx, cmd.CabalID, p.Draft().ID)
	canVote := false
	for _, voter := range voters {
		canVote = canVote || voter == cmd.ProposerID
	}
	return OpenedProposal{
		Draft: p.Draft(), Voters: voters, CreatedAt: now, Needed: rule.Needed(len(voters)), ProposerCanVote: canVote,
		USDCMicros: decimal(p.Draft().USDCMicros.String()), TokenAmount: decimal(p.Draft().TokenAmount.String()),
		QuoteOutAmount: decimal(strconv.FormatUint(p.Draft().QuoteOut, 10)), PriceComparison: a.comparison,
		Stats: Stats{TreasuryShareBps: share, ClosesAt: p.Draft().ExpiresAt},
	}, nil
}

func store(
	ctx context.Context, tx db.Tx, params sqlc.OpenProposalParams, comparison *multichain.Comparison,
	event events.ProposalCreated,
) error {
	const op = "governance.ProposeTrade"
	var err error
	if params.PriceComparison, err = encodeComparison(comparison); err != nil {
		return err
	}
	if _, err := sqlc.New(tx.Queries()).OpenProposal(ctx, params); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return tx.Events.Append(ctx, event)
}

func decimal(raw string) int64 {
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

func shareValue(d domain.Draft) money.Micros {
	if d.Kind == domain.KindSell {
		return money.MicrosFromUint64(d.QuoteOut)
	}
	return d.USDCMicros
}

func (a assessment) potColumn() string {
	if !a.potKnown {
		return ""
	}
	return a.pot.String()
}

type assessment struct {
	pot        money.Micros
	potKnown   bool
	asset      market.Asset
	decimals   uint8
	routed     uint64
	comparison *multichain.Comparison
	priced     *multichain.Comparison
	network    chain.Network
}

func (h *ProposeTradeHandler) assess(ctx context.Context, cmd ProposeTrade) (assessment, error) {
	asset, err := h.ports.Assets.AssetBySymbol(ctx, cmd.Trade.Symbol)
	if err != nil {
		return assessment{}, err
	}
	if cmd.Trade.Kind == domain.KindSell {
		sold, err := h.assessSell(ctx, cmd, asset)
		if err != nil {
			return assessment{}, err
		}
		if pot, err := h.ports.Treasury.PotValue(ctx, cmd.CabalID); err == nil {
			sold.pot, sold.potKnown = pot, true
		}
		return sold, nil
	}
	pot, err := h.ports.Treasury.PotValue(ctx, cmd.CabalID)
	if err != nil {
		return assessment{}, err
	}
	if err := funds(ctx, h.ports, cmd.CabalID, asset, cmd.Trade, pot); err != nil {
		return assessment{}, err
	}
	routed, comparison, err := h.route(ctx, asset, cmd.Trade)
	if err != nil {
		return assessment{}, err
	}
	priced, err := pinned(comparison, cmd.Trade.Network)
	return assessment{
		pot: pot, potKnown: true, asset: asset, decimals: asset.Decimals, routed: routed, comparison: comparison,
		priced: priced, network: cmd.Trade.Network,
	}, err
}

func (h *ProposeTradeHandler) assessSell(
	ctx context.Context, cmd ProposeTrade, asset market.Asset,
) (assessment, error) {
	target, err := h.sellTarget(ctx, cmd.CabalID, asset, cmd.Trade)
	if err != nil {
		return assessment{}, err
	}
	if err := h.sellFunds(ctx, cmd.CabalID, asset, cmd.Trade, target); err != nil {
		return assessment{}, err
	}
	return h.sellRoute(ctx, asset, cmd.Trade, target)
}

func notBanned(ctx context.Context, cabals Cabals, cabal ids.CabalID, op string) error {
	status, err := cabals.Status(ctx, cabal)
	switch {
	case errs.CodeOf(err) == errs.CodeCabalNotFound:
		return nil
	case err == nil && status == cabalport.StatusBanned:
		return errs.New(errs.CodeCabalBanned, op)
	}
	return err
}

func member(ctx context.Context, cabals Cabals, cabal ids.CabalID, user ids.UserID) error {
	ok, err := cabals.IsMember(ctx, cabal, user)
	switch {
	case err != nil:
		return err
	case !ok:
		return errs.New(errs.CodeNotCabalMember, "governance.member")
	}
	return nil
}

func funds(
	ctx context.Context, p TradePorts, cabal ids.CabalID, asset market.Asset, trade domain.Trade, pot money.Micros,
) error {
	if trade.USDCMicros.Cmp(pot) > 0 {
		return errs.New(errs.CodePotExceeded, "governance.funds", slog.String("usdc_micros", trade.USDCMicros.String()))
	}
	cash, cfg, err := spendableCash(ctx, p, cabal, asset)
	if err != nil {
		return err
	}
	if err := cashCovers(trade, cash, cfg); err != nil {
		return err
	}
	if trade.Kind != domain.KindBuy {
		return nil
	}
	rules, err := p.Cabals.Rules(ctx, cabal)
	if err != nil {
		return err
	}
	return withinTradeCap(trade, cash, rules)
}

func cashCovers(trade domain.Trade, cash money.Micros, cfg solana.MintConfig) error {
	need, err := trade.USDCMicros.Add(money.MicrosFromUint64(cfg.FeeHeadroom(trade.USDCMicros.Uint64())))
	if err != nil {
		return err
	}
	if need.Uint64() > cash.Uint64() {
		return errs.New(errs.CodePotCashShort, "governance.funds",
			slog.String("usdc_micros", trade.USDCMicros.String()))
	}
	return nil
}

func withinTradeCap(trade domain.Trade, cash money.Micros, rules cabalport.Rules) error {
	const op = "governance.funds"
	if rules.MaxTradeBps == nil {
		return nil
	}
	if *rules.MaxTradeBps < 0 {
		return errs.New(errs.CodeInternal, op, slog.Int("max_trade_bps", int(*rules.MaxTradeBps)))
	}
	bps := uint64(*rules.MaxTradeBps)
	limit, err := money.MulDiv(cash.Uint64(), bps, bpsScale)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if trade.USDCMicros.Uint64() <= limit {
		return nil
	}
	return errs.NewWithMessage(errs.CodeOverTradeCap, op,
		"This cabal caps a trade at "+formatPercent(bps)+" of its cash ("+formatDollars(limit)+")",
		slog.String("usdc_micros", trade.USDCMicros.String()), slog.Uint64("cap_micros", limit))
}

const (
	bpsScale       = 10_000
	microsPerCent  = 10_000
	centsPerDollar = 100
)

func formatPercent(bps uint64) string {
	whole, frac := bps/100, bps%100
	if frac == 0 {
		return strconv.FormatUint(whole, 10) + "%"
	}
	return strconv.FormatUint(whole, 10) + "." + strings.TrimRight(fmt.Sprintf("%02d", frac), "0") + "%"
}

func formatDollars(micros uint64) string {
	cents := micros / microsPerCent
	digits := strconv.FormatUint(cents/centsPerDollar, 10)
	var grouped []byte
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped = append(grouped, ',')
		}
		grouped = append(grouped, digits[i])
	}
	return fmt.Sprintf("$%s.%02d", grouped, cents%centsPerDollar)
}

func spendableCash(
	ctx context.Context, p TradePorts, cabal ids.CabalID, asset market.Asset,
) (money.Micros, solana.MintConfig, error) {
	wallet, err := p.Cabals.TreasuryWallet(ctx, cabal)
	if err != nil {
		return money.Micros{}, solana.MintConfig{}, err
	}
	onChain, err := p.Balances.TokenBalance(ctx, wallet.Address, p.USDC)
	if err != nil {
		return money.Micros{}, solana.MintConfig{}, err
	}
	cfg, err := p.Balances.MintConfig(ctx, asset.Mint.Address())
	if err != nil {
		return money.Micros{}, solana.MintConfig{}, err
	}
	booked, err := ledgerCash(ctx, p, cabal)
	if err != nil {
		return money.Micros{}, solana.MintConfig{}, err
	}
	cash := money.MicrosFromUint64(onChain.Uint64())
	if booked.Cmp(cash) < 0 {
		cash = booked
	}
	return cash, cfg, nil
}

func ledgerCash(ctx context.Context, p TradePorts, cabal ids.CabalID) (money.Micros, error) {
	positions, err := p.Treasury.SolanaPositions(ctx, cabal)
	if err != nil {
		return money.Micros{}, err
	}
	var cash money.Micros
	for _, position := range positions {
		if position.Mint == p.USDC.Address {
			cash = money.MicrosFromUint64(position.Units.Uint64())
		}
	}
	reserved, err := p.Treasury.CashOutReservations(ctx)
	if err != nil {
		return money.Micros{}, err
	}
	if held := reserved[cabal]; held.Cmp(cash) < 0 {
		return cash.Sub(held)
	}
	return money.Micros{}, nil
}

func (h *ProposeTradeHandler) route(
	ctx context.Context, asset market.Asset, trade domain.Trade,
) (uint64, *multichain.Comparison, error) {
	side, amount := market.SideSell, money.NewBaseUnits(trade.TokenAmount, asset.Decimals)
	if trade.Kind == domain.KindBuy {
		if h.ports.Router != nil {
			comparison, err := h.best(ctx, asset, trade)
			if errs.CodeOf(err) != errs.CodeNoRoute {
				return 0, comparison, err
			}
		}
		side, amount = market.SideBuy, money.NewBaseUnits(trade.USDCMicros.Uint64(), usdcDecimals)
	}
	check, err := h.ports.Routes.CheckRoute(ctx, asset.ID, side, amount)
	return check.OutAmount.Uint64(), nil, err
}

func openParams(d domain.Draft, rule domain.ThresholdRule, voters []ids.UserID, now time.Time) sqlc.OpenProposalParams {
	params := sqlc.OpenProposalParams{
		ID: d.ID.UUID(), CabalID: d.CabalID.UUID(), ProposerID: d.ProposerID.UUID(), Kind: string(d.Kind),
		Symbol: d.Symbol, Mint: string(d.Mint), UsdcMicros: d.USDCMicros.String(), TokenAmount: d.TokenAmount.String(),
		Thesis: d.Thesis, QuoteOutAmount: strconv.FormatUint(d.QuoteOut, 10), Threshold: string(rule),
		ExpiresAt: d.ExpiresAt, CreatedAt: now, Network: string(d.Network),
		VoterIds: make([]uuid.UUID, len(voters)),
	}
	for i, v := range voters {
		params.VoterIds[i] = v.UUID()
	}
	return params
}

func created(d domain.Draft, voters int) events.ProposalCreated {
	return events.ProposalCreated{
		V: 1, ProposalID: d.ID.UUID(), CabalID: d.CabalID.UUID(), ProposerID: d.ProposerID.UUID(),
		Kind: string(d.Kind), Symbol: d.Symbol, Mint: d.Mint, USDCMicros: d.USDCMicros,
		TokenAmount: d.TokenAmount.Uint64(), QuoteOutAmount: d.QuoteOut, ExpiresAt: d.ExpiresAt, VoterCount: voters,
	}
}
