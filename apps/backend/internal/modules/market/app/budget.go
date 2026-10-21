package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type BudgetLevel int

const (
	BudgetOpen BudgetLevel = iota
	BudgetSoft
	BudgetHard
)

const (
	budgetSoftPercent = 80
	budgetHardPercent = 95
)

func (l BudgetLevel) String() string {
	switch l {
	case BudgetOpen:
		return "open"
	case BudgetSoft:
		return "soft"
	case BudgetHard:
		return "hard"
	}
	return "open"
}

type Budget struct {
	reads sqlc.DBTX
	clock clock.Clock
	limit int
}

func NewBudget(db sqlc.DBTX, c clock.Clock, limit int) *Budget {
	return &Budget{reads: db, clock: c, limit: limit}
}

type BudgetUsage struct {
	Month time.Time
	Calls int
	Limit int
	Level BudgetLevel
}

func (b *Budget) month() time.Time {
	now := b.clock.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (b *Budget) threshold(percent int) int { return max(1, b.limit*percent/100) }

func (b *Budget) levelAt(calls int) BudgetLevel {
	switch {
	case b.limit <= 0:
		return BudgetOpen
	case calls >= b.threshold(budgetHardPercent):
		return BudgetHard
	case calls >= b.threshold(budgetSoftPercent):
		return BudgetSoft
	default:
		return BudgetOpen
	}
}

func (b *Budget) Usage(ctx context.Context) (BudgetUsage, error) {
	calls, err := sqlc.New(b.reads).CoinGeckoCalls(ctx, b.clock.Now())
	if err != nil {
		return BudgetUsage{}, errs.Wrap(err, errs.CodeOf(err), "market.Budget.Usage")
	}
	return BudgetUsage{Month: b.month(), Calls: int(calls), Limit: b.limit, Level: b.levelAt(int(calls))}, nil
}

func (b *Budget) Level(ctx context.Context) BudgetLevel {
	if b == nil || b.limit <= 0 {
		return BudgetOpen
	}
	usage, err := b.Usage(ctx)
	if err != nil {
		observability.Degraded(ctx, observability.MarketCoinGeckoUsageFailed, slog.Any("err", err))
		return BudgetHard
	}
	return usage.Level
}

func (b *Budget) Check(ctx context.Context) error {
	const op = "market.Budget.Check"
	if b == nil || b.limit <= 0 {
		return nil
	}
	usage, err := b.Usage(ctx)
	if err != nil {
		observability.Degraded(ctx, observability.MarketCoinGeckoUsageFailed, slog.Any("err", err))
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	if usage.Level == BudgetHard {
		return errs.New(errs.CodeCoinGeckoRateLimited, op, slog.Bool("budget", true))
	}
	return nil
}

func (b *Budget) Reserve(ctx context.Context) error {
	const op = "market.Budget.Reserve"
	if b == nil || b.limit <= 0 {
		return nil
	}
	calls, err := sqlc.New(b.reads).ReserveCoinGeckoCall(ctx, sqlc.ReserveCoinGeckoCallParams{
		At: b.clock.Now(), Hard: int64(b.threshold(budgetHardPercent)),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeCoinGeckoRateLimited, op, slog.Bool("budget", true))
	}
	if err != nil {
		observability.Degraded(ctx, observability.MarketCoinGeckoUsageFailed, slog.Any("err", err))
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	for _, percent := range []int{50, budgetSoftPercent, budgetHardPercent} {
		at := b.threshold(percent)
		if int(calls) == at {
			observability.Degraded(ctx, observability.MarketCoinGeckoBudget,
				slog.Int("percent", percent), slog.Int("calls", int(calls)), slog.Int("limit", b.limit))
		}
	}
	return nil
}
