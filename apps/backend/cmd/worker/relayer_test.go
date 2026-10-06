package main

import (
	"io"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestRun_refusesToBootInProductionWithTheRelayerAtTheFloor(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	err := run(t.Context(), io.Discard, append([]string{
		"MONACO_ENV=production", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"POSTHOG_API_KEY=ph-key", "SOLANA_RPC_URL=" + srv.URL + "/rpc/",
		"RELAYER_PRIVATE_KEY=" + chain.EncodeBase58(fakes.FixtureKey("relayer-at-floor")),
	}, testkit.DeployedEnv()...), noop.NewMeterProvider(), &module.Registry{})
	if errs.CodeOf(err) != errs.CodeRelayerUnderfunded {
		t.Fatalf("run = %v, want relayer_underfunded", err)
	}
}
