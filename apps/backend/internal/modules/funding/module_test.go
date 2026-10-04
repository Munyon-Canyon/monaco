package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModuleTransfersNeedPrivyAndRelayerKeys(t *testing.T) {
	t.Parallel()
	privy := config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()}
	relayer := config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("relayer"))}
	timeouts := config.Timeouts{Privy: time.Second, RPC: time.Second}
	rpc := config.Solana{RPCURL: "http://127.0.0.1/rpc/"}
	for name, c := range map[string]struct {
		cfg config.Config
		ok  bool
	}{
		"no privy key":   {cfg: config.Config{Relayer: relayer, Timeouts: timeouts, Solana: rpc}},
		"no relayer key": {cfg: config.Config{Privy: privy, Timeouts: timeouts, Solana: rpc}},
		"both":           {cfg: config.Config{Privy: privy, Relayer: relayer, Timeouts: timeouts, Solana: rpc}, ok: true},
	} {
		err := funding.New(module.Deps{Config: c.cfg, Clock: testkit.NewClock(time.Time{})}).BuildTransfers()
		if (err == nil) != c.ok {
			t.Errorf("%s: BuildTransfers = %v", name, err)
		}
	}
}
