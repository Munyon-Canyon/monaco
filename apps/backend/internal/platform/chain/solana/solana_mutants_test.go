package solana_test

import (
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMintConfigs_refetchesABatchedMintExactlyAnHourAfterCachingIt(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	mints, fees := batchMints()
	u := &upstream{handler: batchMintHandler(fees)}
	c := solana.New(testConfig(), clk, httpclient.WithTransport(u))
	one := []chain.SolanaAddress{mints[151]}
	fetch := func(want int, when string) {
		t.Helper()
		if _, failures, err := c.MintConfigs(t.Context(), one); err != nil || len(failures) != 0 {
			t.Fatalf("MintConfigs %s = %v, %v", when, failures, err)
		}
		if got := len(u.methods()); got != want {
			t.Fatalf("%d RPC calls %s, want %d", got, when, want)
		}
	}
	fetch(1, "on the first read")
	clk.Advance(time.Hour - time.Nanosecond)
	fetch(1, "one nanosecond inside the hour")
	clk.Advance(time.Nanosecond)
	fetch(2, "at exactly one hour")
}

func TestAccounts_readsExactlyOneHundredAddressesInOneCall(t *testing.T) {
	t.Parallel()
	addrs := make([]chain.SolanaAddress, 100)
	for i := range addrs {
		addrs[i] = member
	}
	u := result(`{"context":{"slot":7},"value":[` + strings.Repeat("null,", 99) + `null]}`)
	slot, got, err := client(u).Accounts(t.Context(), addrs, 0)
	if err != nil || slot != 7 || len(got) != 100 {
		t.Fatalf("Accounts(100 addresses) = %d, %d accounts, %v; want slot 7 and 100 accounts", slot, len(got), err)
	}
	if methods := u.methods(); len(methods) != 1 || methods[0] != "getMultipleAccounts" {
		t.Fatalf("RPC calls = %v, want one getMultipleAccounts", methods)
	}
}
