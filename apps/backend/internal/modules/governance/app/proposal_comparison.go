package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const comparisonTTL = 15 * time.Second

type GetProposalComparison struct {
	CabalID    ids.CabalID
	ProposalID ids.ProposalID
	Caller     ids.UserID
}

type LiveComparison struct {
	QuoteOut   uint64
	Comparison *multichain.Comparison
	QuotedAt   time.Time
}

type ProposalComparisons struct {
	q       *sqlc.Queries
	propose *ProposeTradeHandler
	mu      sync.Mutex
	cached  map[ids.ProposalID]*cachedComparison
}

type cachedComparison struct {
	mu   sync.Mutex
	live LiveComparison
}

func NewProposalComparisons(db sqlc.DBTX, propose *ProposeTradeHandler) *ProposalComparisons {
	return &ProposalComparisons{q: sqlc.New(db), propose: propose, cached: map[ids.ProposalID]*cachedComparison{}}
}

func (c *ProposalComparisons) Get(ctx context.Context, req GetProposalComparison) (LiveComparison, error) {
	const op = "governance.ProposalComparison"
	if err := member(ctx, c.propose.ports.Cabals, req.CabalID, req.Caller); err != nil {
		return LiveComparison{}, err
	}
	row, err := c.q.GetProposal(ctx, sqlc.GetProposalParams{ID: req.ProposalID.UUID(), CallerID: req.Caller.UUID()})
	if err != nil {
		return LiveComparison{}, lookupFailed(err, op)
	}
	if row.CabalID != req.CabalID.UUID() {
		return LiveComparison{}, errs.New(errs.CodeProposalNotFound, op)
	}
	if domain.Kind(row.Kind) != domain.KindBuy || domain.Status(row.Status) != domain.StatusOpen {
		return LiveComparison{}, errs.New(errs.CodeNotFound, op,
			slog.String("kind", row.Kind), slog.String("status", row.Status))
	}
	now := c.propose.clock.Now()
	entry := c.entry(req.ProposalID, now)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if fresh(entry.live, now) {
		return entry.live, nil
	}
	live, err := c.quote(ctx, row, now)
	if err != nil {
		return LiveComparison{}, err
	}
	entry.live = live
	return live, nil
}

func fresh(live LiveComparison, now time.Time) bool {
	return !live.QuotedAt.IsZero() && now.Sub(live.QuotedAt) < comparisonTTL
}

func (c *ProposalComparisons) entry(id ids.ProposalID, now time.Time) *cachedComparison {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.cached[id]; ok {
		return e
	}
	for key, e := range c.cached {
		if e.mu.TryLock() {
			if !fresh(e.live, now) {
				delete(c.cached, key)
			}
			e.mu.Unlock()
		}
	}
	e := &cachedComparison{}
	c.cached[id] = e
	return e
}

func (c *ProposalComparisons) quote(
	ctx context.Context, row sqlc.GetProposalRow, now time.Time,
) (LiveComparison, error) {
	const op = "governance.ProposalComparison"
	asset, err := c.propose.ports.Assets.AssetBySymbol(ctx, row.Symbol)
	if err != nil {
		return LiveComparison{}, err
	}
	trade := domain.Trade{
		Kind: domain.KindBuy, Symbol: row.Symbol, USDCMicros: money.MicrosFromUint64(unsigned(row.UsdcMicros.Int64)),
	}
	routed, err := route(ctx, c.propose.ports.Routes, asset, trade)
	if market.ProbeFailed(err) {
		err = errs.New(errs.CodeNoRoute, op)
	}
	if err != nil {
		return LiveComparison{}, err
	}
	comparison := c.propose.compare(ctx, asset, trade)
	observability.Debug(ctx, observability.GovernanceProposalComparisonRefreshed,
		slog.String("proposal_id", row.ID.String()), slog.Bool("compared", comparison != nil))
	return LiveComparison{
		QuoteOut:   displayQuote(asset, comparison, routed.OutAmount.Uint64(), now),
		Comparison: comparison,
		QuotedAt:   now,
	}, nil
}
