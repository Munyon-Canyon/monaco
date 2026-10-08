package app

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
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
}

type Assets interface {
	AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error)
}

type Treasury interface {
	PotValue(ctx context.Context, id ids.CabalID) (money.Micros, error)
	Positions(ctx context.Context, id ids.CabalID) ([]treasuryport.Position, error)
}

type Balances interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
}

type TradePorts struct {
	Cabals   Cabals
	Assets   Assets
	Routes   market.Routes
	Treasury Treasury
	Balances Balances
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
	asset, quote, err := h.assess(ctx, cmd)
	if err != nil {
		return OpenedProposal{}, err
	}
	now := h.clock.Now()
	p, err := domain.NewProposal(domain.Draft{
		ID: ids.ProposalIDFrom(h.ids.NewV7()), CabalID: cmd.CabalID, ProposerID: cmd.ProposerID, Kind: cmd.Trade.Kind,
		Symbol: asset.Symbol, Mint: asset.Mint.Address(), USDCMicros: cmd.Trade.USDCMicros,
		TokenAmount: money.NewBaseUnits(cmd.Trade.TokenAmount, asset.Decimals), Thesis: cmd.Trade.Thesis,
		QuoteOut: quote.OutAmount.Uint64(), ExpiresAt: now.Add(rules.ProposalExpiry),
	})
	if err != nil {
		return OpenedProposal{}, err
	}
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if _, err := sqlc.New(tx.Queries()).OpenProposal(ctx, openParams(p.Draft(), rule, voters, now)); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		return tx.Events.Append(ctx, created(p.Draft(), len(voters)))
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
		QuoteOutAmount: decimal(strconv.FormatUint(p.Draft().QuoteOut, 10)),
	}, nil
}

func decimal(raw string) int64 {
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

func (h *ProposeTradeHandler) assess(ctx context.Context, cmd ProposeTrade) (market.Asset, market.RouteCheck, error) {
	asset, err := h.ports.Assets.AssetBySymbol(ctx, cmd.Trade.Symbol)
	if err != nil {
		return market.Asset{}, market.RouteCheck{}, err
	}
	var pot money.Micros
	if cmd.Trade.Kind == domain.KindBuy {
		if pot, err = h.ports.Treasury.PotValue(ctx, cmd.CabalID); err != nil {
			return market.Asset{}, market.RouteCheck{}, err
		}
	}
	if err := funds(ctx, h.ports, cmd.CabalID, asset, cmd.Trade, pot); err != nil {
		return market.Asset{}, market.RouteCheck{}, err
	}
	quote, err := route(ctx, h.ports.Routes, asset, cmd.Trade)
	if err != nil {
		return market.Asset{}, market.RouteCheck{}, err
	}
	return asset, quote, nil
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
	const op = "governance.funds"
	if trade.Kind == domain.KindBuy {
		if trade.USDCMicros.Cmp(pot) > 0 {
			return errs.New(errs.CodePotExceeded, op, slog.String("usdc_micros", trade.USDCMicros.String()))
		}
		return nil
	}
	if err := ledgerHolds(ctx, p.Treasury, cabal, asset, trade); err != nil {
		return err
	}
	wallet, err := p.Cabals.TreasuryWallet(ctx, cabal)
	if err != nil {
		return err
	}
	mint := chain.Mint{Address: asset.Mint.Address(), Decimals: asset.Decimals}
	have, err := p.Balances.TokenBalance(ctx, wallet.Address, mint)
	if err != nil {
		return err
	}
	if have.Uint64() < trade.TokenAmount {
		return errs.New(errs.CodeInsufficientFunds, op, slog.String("symbol", asset.Symbol))
	}
	return nil
}

func ledgerHolds(
	ctx context.Context, t Treasury, cabal ids.CabalID, asset market.Asset, trade domain.Trade,
) error {
	const op = "governance.funds"
	positions, err := t.Positions(ctx, cabal)
	if err != nil {
		return err
	}
	want := money.NewBaseUnits(trade.TokenAmount, asset.Decimals)
	for _, p := range positions {
		if p.Mint != asset.Mint.Address() {
			continue
		}
		short, err := p.Units.Cmp(want)
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if short >= 0 {
			return nil
		}
	}
	return errs.New(errs.CodeInsufficientFunds, op, slog.String("symbol", asset.Symbol))
}

func route(
	ctx context.Context, routes market.Routes, asset market.Asset, trade domain.Trade,
) (market.RouteCheck, error) {
	if trade.Kind == domain.KindBuy {
		return routes.CheckRoute(
			ctx,
			asset.ID,
			market.SideBuy,
			money.NewBaseUnits(trade.USDCMicros.Uint64(), usdcDecimals),
		)
	}
	return routes.CheckRoute(ctx, asset.ID, market.SideSell, money.NewBaseUnits(trade.TokenAmount, asset.Decimals))
}

func openParams(d domain.Draft, rule domain.ThresholdRule, voters []ids.UserID, now time.Time) sqlc.OpenProposalParams {
	params := sqlc.OpenProposalParams{
		ID: d.ID.UUID(), CabalID: d.CabalID.UUID(), ProposerID: d.ProposerID.UUID(), Kind: string(d.Kind),
		Symbol: d.Symbol, Mint: string(d.Mint), UsdcMicros: d.USDCMicros.String(), TokenAmount: d.TokenAmount.String(),
		Thesis: d.Thesis, QuoteOutAmount: strconv.FormatUint(d.QuoteOut, 10), Threshold: string(rule),
		ExpiresAt: d.ExpiresAt, CreatedAt: now,
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
