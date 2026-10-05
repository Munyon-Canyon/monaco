package app

import (
	"context"
	"sync"
	"time"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const TreasuryMapTTL = 60 * time.Second

type TreasuryWallets interface {
	TreasuryWallets(ctx context.Context) ([]cabalport.TreasuryWallet, error)
}

type TreasuryMap struct {
	wallets TreasuryWallets
	clock   clock.Clock

	mu       sync.Mutex
	byAddr   map[chain.SolanaAddress]ids.CabalID
	loadedAt time.Time
}

func NewTreasuryMap(wallets TreasuryWallets, c clock.Clock) *TreasuryMap {
	return &TreasuryMap{wallets: wallets, clock: c}
}

func (m *TreasuryMap) CabalFor(ctx context.Context, addr chain.SolanaAddress) (ids.CabalID, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byAddr[addr]
	if ok && m.clock.Now().Sub(m.loadedAt) < TreasuryMapTTL {
		return id, true, nil
	}
	wallets, err := m.wallets.TreasuryWallets(ctx)
	if err != nil {
		return ids.CabalID{}, false, err
	}
	m.byAddr = make(map[chain.SolanaAddress]ids.CabalID, len(wallets))
	for _, w := range wallets {
		m.byAddr[w.Address] = w.CabalID
	}
	m.loadedAt = m.clock.Now()
	id, ok = m.byAddr[addr]
	return id, ok, nil
}
