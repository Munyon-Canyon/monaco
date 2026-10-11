package trading

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLazyArrivals_readsThroughTheConfiguredSolanaRPC(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	cfg.Solana.RPCURL, cfg.Timeouts.RPC = "http://127.0.0.1:1", time.Second
	a := lazyArrivals{m: &Module{deps: module.Deps{Config: cfg, Clock: testkit.NewClock(clock.Real{}.Now())}}}

	_, err := a.InboundAmount(
		t.Context(),
		"sig",
		"6HUUQBkqjMRGWuqnp1t6wB8iHRKhY15pcTAq47ZX8ELx",
		chain.Mint{Decimals: 6},
	)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("InboundAmount err = %v, want rpc_unavailable from the unreachable RPC", err)
	}
}
