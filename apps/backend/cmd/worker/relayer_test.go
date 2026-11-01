package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestRun_aLowRelayerAlertsAndDoesNotStopTheWorkerBooting(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	var stderr bytes.Buffer
	err := run(t.Context(), &stderr, append(testkit.DeployedEnv(),
		"MONACO_ENV=production", "DATABASE_URL=postgres://127.0.0.1:1/monaco", "NATS_URL=nats://127.0.0.1:1",
		"POSTHOG_API_KEY=ph-key", "SOLANA_RPC_URL="+srv.URL+"/rpc/",
		"RELAYER_PRIVATE_KEY="+chain.EncodeBase58(fakes.FixtureKey("relayer-at-floor")),
	), noop.NewMeterProvider(), &module.Registry{})
	if err == nil || errs.CodeOf(err) == errs.CodeRelayerUnderfunded {
		t.Fatalf("run = %v, want it to get past the relayer check and stop at the database", err)
	}
	if !strings.Contains(stderr.String(), "relayer.boot_underfunded") {
		t.Fatalf("no relayer.boot_underfunded alert:\n%s", stderr.String())
	}
}

func TestRun_refusesToBootWithoutARelayerKey(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	err := run(t.Context(), &bytes.Buffer{}, append(testkit.DeployedEnv(),
		"MONACO_ENV=production", "DATABASE_URL=postgres://127.0.0.1:1/monaco", "NATS_URL=nats://127.0.0.1:1",
		"POSTHOG_API_KEY=ph-key", "SOLANA_RPC_URL="+srv.URL+"/rpc/",
	), noop.NewMeterProvider(), &module.Registry{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run = %v, want invalid_input from the missing relayer key", err)
	}
}
