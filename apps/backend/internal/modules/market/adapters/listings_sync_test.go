package adapters

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNewListingSyncRunsOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	var named []string
	deps := module.Deps{
		Clock: clock.Real{}, IDs: testkit.NewIDs(4969),
		HTTPClient: func(name string, opts ...httpclient.Option) *httpclient.Client {
			named = append(named, name)
			return httpclient.New(name, opts...)
		},
	}
	xs := xstocks.New(httpclient.New("xstocks", httpclient.WithTimeout(time.Second)))
	if NewListingSync(deps, xs) != nil || len(named) != 0 {
		t.Fatalf("a disabled sync was built with clients %v", named)
	}
	deps.Config = config.Config{
		Listings: config.Listings{
			SyncEnabled: true, OndoBaseURL: "http://ondo.invalid", CoinbaseBaseURL: "http://coinbase.invalid",
			RobinhoodBaseURL: "http://robinhood.invalid",
		}, Timeouts: config.Timeouts{RPC: time.Second}, Solana: config.Solana{RPCURL: "http://solana.invalid"},
	}
	if NewListingSync(deps, xs) == nil {
		t.Fatal("an enabled sync was not built")
	}
	if want := []string{"ondo", "coinbase", "robinhood", "evm-rpc"}; len(named) != len(want) {
		t.Fatalf("clients = %v, want %v", named, want)
	}
}
