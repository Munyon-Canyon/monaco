package trading_test

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type chainFills struct {
	app.ChainReader

	mu     sync.Mutex
	landed map[platform.Signature]uint64
	feeBps uint64
	err    error
	reads  []platform.Mint
}

func newChainFills() *chainFills { return &chainFills{landed: map[platform.Signature]uint64{}} }

func (f *chainFills) InboundAmount(
	_ context.Context, sig platform.Signature, _ platform.SolanaAddress, mint platform.Mint,
) (money.BaseUnits, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, mint)
	if f.err != nil {
		return money.BaseUnits{}, f.err
	}
	out, ok := f.landed[sig]
	if !ok {
		return money.BaseUnits{}, errs.New(errs.CodeNotFound, "chainFills.InboundAmount")
	}
	return money.NewBaseUnits(out-out*f.feeBps/10_000, mint.Decimals), nil
}

func (f *chainFills) charge(bps uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.feeBps = bps
}

func (f *chainFills) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *chainFills) mints() []platform.Mint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]platform.Mint(nil), f.reads...)
}

type landingVenue struct {
	app.Venue
	fills *chainFills
}

func (v landingVenue) ExecuteUntilTerminal(
	ctx context.Context,
	requestID string,
	signed []byte,
) (app.ExecuteResult, error) {
	r, err := v.Venue.ExecuteUntilTerminal(ctx, requestID, signed)
	if err == nil && r.Status == app.ExecuteSuccess && len(signed) > 64 {
		v.fills.mu.Lock()
		v.fills.landed[platform.SignatureOf(signed[1:65])] = r.OutAmount
		v.fills.mu.Unlock()
	}
	return r, err
}
