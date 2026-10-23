package trading

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestLazyTables_readsTablesThroughTheConfiguredRPC(t *testing.T) {
	t.Parallel()
	entry := chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	raw, _ := entry.Bytes()
	data := base64.StdEncoding.EncodeToString(append(make([]byte, 56), raw...))
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"context":{"slot":1},"value":[{"owner":"` +
			string(solana.LookupTableProgram) + `","data":["` + data + `","base64"]}]}}`))
	}))
	t.Cleanup(rpc.Close)
	tables := &lazyTables{
		cfg: config.Config{Solana: config.Solana{RPCURL: rpc.URL}, Timeouts: config.Timeouts{RPC: time.Second}},
		clk: clock.Real{},
	}
	table := chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")

	got, err := tables.LookupTables(t.Context(), []chain.SolanaAddress{table})

	if err != nil || len(got[table]) != 1 || got[table][0] != entry {
		t.Fatalf("LookupTables = %+v, %v, want the one entry the table holds", got, err)
	}
}
