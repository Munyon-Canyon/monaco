package relayer

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LazyTopUps struct {
	once   sync.Once
	build  func() (*TopUps, error)
	topUps *TopUps
	err    error
}

func NewLazyTopUps(cfg config.Config, uow *db.UnitOfWork, clk clock.Clock) *LazyTopUps {
	return &LazyTopUps{build: func() (*TopUps, error) {
		rpc := solana.New(cfg, clk)
		r, err := New(cfg, rpc)
		if err != nil {
			return nil, err
		}
		return NewTopUps(r, rpc, uow, clk, ids.Real{}), nil
	}}
}

func (l *LazyTopUps) TopUpSOL(ctx context.Context, wallet chain.SolanaAddress) error {
	l.once.Do(func() { l.topUps, l.err = l.build() })
	if l.err != nil {
		return l.err
	}
	return l.topUps.TopUpSOL(ctx, wallet)
}
