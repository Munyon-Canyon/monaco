package adapters_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func codeServer(t *testing.T, reply string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestEVMCode_readsEachConfiguredChainsCode(t *testing.T) {
	t.Parallel()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	code := adapters.NewEVMCode(module.Deps{Config: config.Config{
		Timeouts: config.Timeouts{RPC: time.Second},
		EVM: config.EVM{
			RPCURLBase:      codeServer(t, `{"jsonrpc":"2.0","id":1,"result":"0xef0100aa"}`),
			RPCURLRobinhood: codeServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"down"}}`),
			RPCURLEthereum:  closed.URL,
		},
	}})
	account, _ := chain.ParseEVMAddress("0x00000000000000000000000000000000000051a1")
	if got := code.Networks(); !slices.Equal(got,
		[]chain.Network{chain.NetworkBase, chain.NetworkEthereum, chain.NetworkRobinhood}) {
		t.Fatalf("networks = %v, want the chains with an RPC URL in order", got)
	}
	if got, err := code.Code(t.Context(), chain.NetworkBase, account); err != nil || got != "0xef0100aa" {
		t.Fatalf("base = %q, %v", got, err)
	}
	for n, want := range map[chain.Network]errs.Code{
		chain.NetworkRobinhood: errs.CodeRPCUnavailable,
		chain.NetworkEthereum:  errs.CodeRPCUnavailable,
		chain.NetworkArbitrum:  errs.CodeNoRoute,
	} {
		if got, err := code.Code(t.Context(), n, account); errs.CodeOf(err) != want || got != "" {
			t.Fatalf("%s = %q, %v, want %s", n, got, err, want)
		}
	}
}
