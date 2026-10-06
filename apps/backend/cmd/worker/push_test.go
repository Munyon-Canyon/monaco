package main

import (
	"io"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func pushConfig(t *testing.T, environ ...string) config.Config {
	t.Helper()
	cfg, err := config.Load(append([]string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
	}, environ...))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestBindPush_withoutAKeyBindsTheNoopSenderAndSaysSo(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	d := module.Deps{Config: pushConfig(t)}

	if err := bindPush(ctx, &d); err != nil {
		t.Fatal(err)
	}

	if _, ok := d.APNs.(apns.NoopSender); !ok {
		t.Fatalf("APNs = %T, want apns.NoopSender", d.APNs)
	}
	if line := string(logs.Bytes()); !strings.Contains(line, `"msg":"boot.push_disabled"`) ||
		!strings.Contains(line, `"service":"worker"`) {
		t.Fatalf("boot log = %s, want boot.push_disabled from the worker", line)
	}
}

func TestBindPush_withAKeyBindsTheRealClientAndStaysQuiet(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	d := module.Deps{Config: pushConfig(t, testkit.DeployedEnv()...)}

	if err := bindPush(ctx, &d); err != nil {
		t.Fatal(err)
	}

	if _, ok := d.APNs.(*apns.Client); !ok {
		t.Fatalf("APNs = %T, want *apns.Client", d.APNs)
	}
	if len(logs.Bytes()) != 0 {
		t.Fatalf("boot log = %s, want nothing when push is enabled", logs.Bytes())
	}
}

func TestBindPush_refusesAKeyItCannotUse(t *testing.T) {
	t.Parallel()
	d := module.Deps{Config: pushConfig(t, "APNS_KEY_P8=not a key", "APNS_KEY_ID=key", "APNS_TEAM_ID=team")}

	err := bindPush(t.Context(), &d)

	if errs.CodeOf(err) != errs.CodeInvalidConfig || d.APNs != nil {
		t.Fatalf("bindPush = %v with APNs %T, want invalid_config and nothing bound", err, d.APNs)
	}
}

func TestRun_refusesToBootWithPartialStorageConfig(t *testing.T) {
	t.Parallel()
	env := bootEnv(t, "SUPABASE_URL=https://storage.example", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0")

	err := run(t.Context(), io.Discard, env, noop.NewMeterProvider(), &module.Registry{})

	if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.HasPrefix(err.Error(), "storage.New: ") {
		t.Fatalf("run = %v, want invalid_config from storage.New", err)
	}
}

func TestRun_refusesToBootWithAnAPNsKeyItCannotUse(t *testing.T) {
	t.Parallel()
	env := bootEnv(
		t,
		"APNS_KEY_P8=not a key",
		"APNS_KEY_ID=key",
		"APNS_TEAM_ID=team",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
	)

	err := run(t.Context(), io.Discard, env, noop.NewMeterProvider(), &module.Registry{})

	if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.HasPrefix(err.Error(), "apns.New: ") {
		t.Fatalf("run = %v, want invalid_config from apns.New", err)
	}
}
