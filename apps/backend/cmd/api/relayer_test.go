package main

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestPreflight_bootsInStagingWithTheRelayerAtTheFloorAndAlerts(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	cfg, err := load(append(testkit.DeployedEnv(),
		"MONACO_ENV=staging", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_DEV_TOKEN_KEY=dev-only", "SOLANA_RPC_URL="+srv.URL+"/rpc/",
		"RELAYER_PRIVATE_KEY="+chain.EncodeBase58(fakes.FixtureKey("relayer-at-floor")),
	))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewTextHandler(&logs, nil)))
	if err := preflight(ctx, cfg); err != nil {
		t.Fatalf("preflight = %v, want the API to boot on a low relayer", err)
	}
	if !strings.Contains(logs.String(), "msg=relayer.boot_underfunded") {
		t.Fatalf("no relayer.boot_underfunded alert:\n%s", logs.String())
	}
}
