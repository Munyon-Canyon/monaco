package main

import (
	"bytes"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func relayerEnviron(rpcURL, key string) []string {
	return []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://unused/monaco", "NATS_URL=nats://unused",
		"SOLANA_RPC_URL=" + rpcURL, "RELAYER_PRIVATE_KEY=" + key,
	}
}

func TestRelayerBalance_printsThePubkeyAndItsSOLFromTheRPC(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	key := fakes.FixtureKey("relayer-balance")
	var stdout, stderr bytes.Buffer
	code := relayerTool(relayerEnviron(srv.URL+"/rpc/", chain.EncodeBase58(key)))([]string{"balance"}, &stdout, &stderr)
	want := "relayer " + string(
		chain.AddressOf(key.Public().(ed25519.PublicKey)),
	) + "\nbalance 0.050000000 SOL (50000000 lamports)\n"
	if code != 0 || stdout.String() != want {
		t.Fatalf("relayer balance = %d, stdout %q, stderr %q, want %q", code, stdout.String(), stderr.String(), want)
	}
}

func TestRelayerBalance_refusesBadArgsAKeyItCannotReadAndAnRPCError(t *testing.T) {
	t.Parallel()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	goodKey := chain.EncodeBase58(fakes.FixtureKey("relayer-balance"))
	for _, tc := range []struct {
		name    string
		environ []string
		args    []string
		want    int
	}{
		{"no subcommand", relayerEnviron(down.URL, goodKey), nil, 2},
		{"unknown subcommand", relayerEnviron(down.URL, goodKey), []string{"fund"}, 2},
		{"bad config", []string{"MONACO_ENV=nowhere"}, []string{"balance"}, 1},
		{"bad key", relayerEnviron(down.URL, "not-a-key"), []string{"balance"}, 1},
		{"rpc down", relayerEnviron(down.URL, goodKey), []string{"balance"}, 1},
	} {
		var stdout, stderr bytes.Buffer
		if code := relayerTool(tc.environ)(tc.args, &stdout, &stderr); code != tc.want || stdout.Len() != 0 {
			t.Errorf("%s: code %d stdout %q stderr %q, want %d and no stdout", tc.name, code, stdout.String(),
				stderr.String(), tc.want)
		}
	}
}
