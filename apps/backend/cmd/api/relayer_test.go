package main

import (
	"io"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestRun_refusesToBootInStagingWithTheRelayerAtTheFloor(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	err := run(t.Context(), io.Discard, append([]string{
		"MONACO_ENV=staging", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_DEV_TOKEN_KEY=dev-only", "SOLANA_RPC_URL=" + srv.URL + "/rpc/",
		"RELAYER_PRIVATE_KEY=" + chain.EncodeBase58(fakes.FixtureKey("relayer-at-floor")),
	}, testkit.APNsEnv()...), openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeRelayerUnderfunded {
		t.Fatalf("run = %v, want relayer_underfunded", err)
	}
}
