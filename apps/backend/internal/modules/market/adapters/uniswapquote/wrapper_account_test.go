package uniswapquote_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	storedFirst  = "0x00000000000000000000000000000000000051a1"
	storedSecond = "0x00000000000000000000000000000000000051a2"
)

func storedClient(t *testing.T, r *wrapperRig, stored *uniswapquote.StoredAccount) *uniswapquote.Client {
	t.Helper()
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	rpcs := map[chain.Network]*httpclient.Client{chain.NetworkBase: httpclient.New("evm-rpc-stored",
		httpclient.WithBaseURL(r.rpcURL), httpclient.WithTimeout(time.Second))}
	hc := httpclient.New("uniswap-stored", uniswapquote.Options(r.uniURL(t), 5*time.Second)...)
	return uniswapquote.New(hc, "test-key", stored, r.clock).WithWrapper(uniswapquote.NewWrapper(rpcs, delegate))
}

func (r *wrapperRig) lastAccount() (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	params, _ := r.calls[len(r.calls)-1]["params"].([]any)
	call, _ := params[0].(map[string]any)
	to, _ := call["to"].(string)
	swapper, _ := r.quotes[len(r.quotes)-1]["swapper"].(string)
	return swapper, to
}

func TestWrapper_quotesAndSimulatesFromTheStoredAccount(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	first, _ := chain.ParseEVMAddress(storedFirst)
	stored := uniswapquote.Stored(first)
	client := storedClient(t, r, stored)
	buy := func(micros uint64) {
		t.Helper()
		if _, err := client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy,
			money.MicrosFromUint64(micros)); err != nil {
			t.Fatalf("Quote: %v", err)
		}
	}

	buy(liveInput)
	if swapper, simulated := r.lastAccount(); swapper != storedFirst || simulated != storedFirst {
		t.Fatalf("swapper %s, simulated from %s, want the stored %s", swapper, simulated, storedFirst)
	}
	stored.Address, _ = chain.ParseEVMAddress(storedSecond)
	buy(liveInput + 1)
	if swapper, simulated := r.lastAccount(); swapper != storedSecond || simulated != storedSecond {
		t.Fatalf("swapper %s, simulated from %s after the store changed, want %s", swapper, simulated, storedSecond)
	}
}

func TestWrapper_dropsTheVenueWithoutAStoredAccount(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	for want, stored := range map[errs.Code]*uniswapquote.StoredAccount{
		errs.CodeNoRoute:  {},
		errs.CodeInternal: {Err: errs.New(errs.CodeInternal, "fake")},
	} {
		_, err := storedClient(t, r, stored).Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy,
			money.MicrosFromUint64(liveInput))
		if errs.CodeOf(err) != want || stored.Reads != 1 || len(r.quotes) != 0 {
			t.Fatalf("err %v, reads %d, quotes %d, want %s before any upstream call", err, stored.Reads,
				len(r.quotes), want)
		}
	}
}
