package adapters

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var (
	_ mc.Router        = (*LazyRouter)(nil)
	_ mc.NetworkRouter = (*LazyRouter)(nil)
)

type LazyRouter struct {
	once   sync.Once
	build  func() mc.Router
	router mc.Router
}

func NewLazyRouter(build func() mc.Router) *LazyRouter { return &LazyRouter{build: build} }

func (l *LazyRouter) Best(
	ctx context.Context, stock mc.StockID, side mc.Side, amount money.Micros,
) (mc.Comparison, error) {
	l.once.Do(func() { l.router = l.build() })
	return l.router.Best(ctx, stock, side, amount)
}

func (l *LazyRouter) BestOn(
	ctx context.Context, stock mc.StockID, side mc.Side, amount money.Micros, n chain.Network,
) (mc.Comparison, error) {
	l.once.Do(func() { l.router = l.build() })
	return mc.BestOn(ctx, l.router, stock, side, amount, n)
}
