package trading_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_isNamedTradingAndServesNothingYet(t *testing.T) {
	t.Parallel()
	m := trading.New(module.Deps{Config: config.Config{
		Solana: config.Solana{RPCURL: "http://fakes/rpc/"}, Timeouts: config.Timeouts{RPC: time.Second},
	}})
	if m.Name() != "trading" || len(m.Consumers()) != 0 || len(m.Pollers()) != 1 ||
		testkit.Serves(m.Mount, "GET", "/v1/assets") {
		t.Fatalf("module = %s, %d consumers, %v pollers", m.Name(), len(m.Consumers()), m.Pollers())
	}
	if reflect.ValueOf(m.SignatureOwner()).IsZero() {
		t.Fatal("SignatureOwner() = zero value")
	}
}
