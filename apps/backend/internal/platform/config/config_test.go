package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func required() []string {
	return []string{
		"MONACO_ENV=local",
		"DATABASE_URL=postgres://monaco@localhost:54322/monaco",
		"NATS_URL=nats://localhost:4222",
	}
}

func TestLoadFillsDefaultsFromTheRFC(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "PATH=/usr/bin", "HOME=/home/monaco"))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Env:    config.EnvLocal,
		HTTP:   config.HTTP{Addr: ":8080", MaxBodyBytes: 1 << 20},
		Worker: config.Worker{HealthAddr: ":8081"},
		DB:     config.DB{URL: "postgres://monaco@localhost:54322/monaco", MaxConns: 10},
		NATS:   config.NATS{URL: "nats://localhost:4222"},
		OTel:   config.OTel{ServiceName: "monaco"},
		Timeouts: config.Timeouts{
			RPC:             5 * time.Second,
			Privy:           10 * time.Second,
			JupiterQuote:    5 * time.Second,
			JupiterExecute:  2 * time.Minute,
			HTTPServerRead:  10 * time.Second,
			HTTPServerWrite: 30 * time.Second,
			Shutdown:        10 * time.Second,
		},
		Jupiter: config.Jupiter{SwapBaseURL: "https://api.jup.ag/swap/v2", PriceBaseURL: "https://api.jup.ag/price/v3"},
		Privy:   config.Privy{BaseURL: "https://api.privy.io"},
		Solana: config.Solana{
			RPCURL:   "https://api.mainnet-beta.solana.com",
			USDCMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadReadsEveryKey(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load([]string{
		"MONACO_ENV=production",
		"MONACO_HTTP_ADDR=127.0.0.1:9000",
		"MONACO_HTTP_MAX_BODY_BYTES=4096",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:9001",
		"DATABASE_URL=postgres://prod",
		"MONACO_DB_MAX_CONNS=40",
		"NATS_URL=nats://prod:4222",
		"OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.example",
		"OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic abc",
		"OTEL_SERVICE_NAME=monaco-api",
		"MONACO_DEV_TOKEN_KEY=dev-secret",
		"MONACO_TIMEOUT_RPC=1s",
		"MONACO_TIMEOUT_PRIVY=2s",
		"MONACO_TIMEOUT_JUPITER_QUOTE=3s",
		"MONACO_TIMEOUT_JUPITER_EXECUTE=4m",
		"MONACO_TIMEOUT_HTTP_SERVER_READ=5s",
		"MONACO_TIMEOUT_HTTP_SERVER_WRITE=6s",
		"MONACO_TIMEOUT_SHUTDOWN=7s",
		"MONACO_JUPITER_SWAP_BASE_URL=http://fakes/jupiter/swap/v2",
		"MONACO_JUPITER_PRICE_BASE_URL=http://fakes/jupiter/price/v3",
		"JUPITER_API_KEY=jup-secret",
		"PRIVY_APP_ID=app-id",
		"PRIVY_APP_SECRET=app-secret",
		"PRIVY_VERIFICATION_KEY=verification-pem",
		"PRIVY_AUTHORIZATION_PRIVATE_KEY=wallet-auth:key",
		"PRIVY_AUTHORIZATION_KEY_ID=quorum-id",
		"PRIVY_WEBHOOK_SECRET=whsec_x",
		"PRIVY_BASE_URL=http://fakes/privy",
		"SOLANA_RPC_URL=http://fakes/rpc",
		"SOLANA_USDC_MINT=mint",
		"RELAYER_PRIVATE_KEY=relayer-key",
		"MONACO_FAULTPOINT=before-commit",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Env:    config.EnvProduction,
		HTTP:   config.HTTP{Addr: "127.0.0.1:9000", MaxBodyBytes: 4096},
		Worker: config.Worker{HealthAddr: "127.0.0.1:9001"},
		DB:     config.DB{URL: "postgres://prod", MaxConns: 40},
		NATS:   config.NATS{URL: "nats://prod:4222"},
		OTel: config.OTel{
			Endpoint:    "https://otlp.example",
			Headers:     "Authorization=Basic abc",
			ServiceName: "monaco-api",
		},
		Auth: config.Auth{DevTokenKey: "dev-secret"},
		Timeouts: config.Timeouts{
			RPC:             time.Second,
			Privy:           2 * time.Second,
			JupiterQuote:    3 * time.Second,
			JupiterExecute:  4 * time.Minute,
			HTTPServerRead:  5 * time.Second,
			HTTPServerWrite: 6 * time.Second,
			Shutdown:        7 * time.Second,
		},
		Jupiter: config.Jupiter{
			SwapBaseURL:  "http://fakes/jupiter/swap/v2",
			PriceBaseURL: "http://fakes/jupiter/price/v3",
			APIKey:       "jup-secret",
		},
		Privy: config.Privy{
			AppID: "app-id", AppSecret: "app-secret", VerificationKey: "verification-pem",
			AuthorizationPrivateKey: "wallet-auth:key", AuthorizationKeyID: "quorum-id",
			WebhookSecret: "whsec_x", BaseURL: "http://fakes/privy",
		},
		Solana:     config.Solana{RPCURL: "http://fakes/rpc", USDCMint: "mint"},
		Relayer:    config.Relayer{PrivateKey: "relayer-key"},
		Faultpoint: "before-commit",
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadAcceptsEveryEnv(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging, config.EnvProduction} {
		cfg, err := config.Load(append(required(), "MONACO_ENV="+string(env)))
		if err != nil {
			t.Fatalf("MONACO_ENV=%s: %v", env, err)
		}
		if cfg.Env != env {
			t.Fatalf("Env = %q, want %q", cfg.Env, env)
		}
	}
}

func TestLoadFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		environ []string
		want    string
	}{
		{
			name:    "nothing set names every required key",
			environ: []string{"PATH=/usr/bin"},
			want:    "config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL",
		},
		{
			name:    "missing database and nats in one error",
			environ: []string{"MONACO_ENV=local"},
			want:    "config.Load: invalid_input: missing DATABASE_URL, NATS_URL",
		},
		{
			name:    "empty value counts as missing",
			environ: append(required(), "DATABASE_URL="),
			want:    "config.Load: invalid_input: missing DATABASE_URL",
		},
		{
			name:    "unknown MONACO key",
			environ: append(required(), "MONACO_FOO=1", "MONACO_BAR", "FOO=1"),
			want:    "config.Load: invalid_input: unknown MONACO_BAR, MONACO_FOO",
		},
		{
			name:    "env outside the four names",
			environ: append(required(), "MONACO_ENV=prod"),
			want:    "config.Load: invalid_input: invalid MONACO_ENV (local, test, staging or production)",
		},
		{
			name: "malformed numbers and durations",
			environ: append(required(),
				"MONACO_DB_MAX_CONNS=0",
				"MONACO_TIMEOUT_RPC=5",
				"MONACO_TIMEOUT_PRIVY=-1s",
				"MONACO_TIMEOUT_SHUTDOWN=0s",
			),
			want: "config.Load: invalid_input: invalid MONACO_DB_MAX_CONNS (positive integer), " +
				"MONACO_TIMEOUT_RPC (positive duration like 5s), MONACO_TIMEOUT_PRIVY (positive duration like 5s), " +
				"MONACO_TIMEOUT_SHUTDOWN (positive duration like 5s)",
		},
		{
			name:    "max conns past int32",
			environ: append(required(), "MONACO_DB_MAX_CONNS=2147483648"),
			want:    "config.Load: invalid_input: invalid MONACO_DB_MAX_CONNS (positive integer)",
		},
		{
			name:    "every kind of problem at once",
			environ: []string{"MONACO_ENV=local", "NATS_URL=nats://x", "MONACO_FOO=1", "MONACO_DB_MAX_CONNS=x"},
			want: "config.Load: invalid_input: missing DATABASE_URL; unknown MONACO_FOO; " +
				"invalid MONACO_DB_MAX_CONNS (positive integer)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.Load(tt.environ)
			if err == nil {
				t.Fatalf("Load = %+v, want error %q", cfg, tt.want)
			}
			if err.Error() != tt.want {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
			if target := (*errs.Error)(nil); !errors.As(err, &target) || errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("error %v is not an *errs.Error with code invalid_input", err)
			}
			if !reflect.DeepEqual(cfg, config.Config{}) {
				t.Fatalf("Load returned %+v with an error, want the zero Config", cfg)
			}
		})
	}
}

func TestLoadErrorNeverEchoesAValue(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-value"
	_, err := config.Load([]string{"DATABASE_URL=" + secret, "MONACO_ENV=" + secret, "MONACO_TIMEOUT_RPC=" + secret})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error %v echoes a value", err)
	}
}

func TestRedactedHidesSecretsAndShowsTheRest(t *testing.T) {
	t.Parallel()
	secrets := map[string]string{
		"DATABASE_URL":                    "postgres://db-secret@host/db",
		"NATS_URL":                        "nats://token-secret@host:4222",
		"OTEL_EXPORTER_OTLP_HEADERS":      "Authorization=Basic header-secret",
		"MONACO_DEV_TOKEN_KEY":            "dev-token-secret",
		"JUPITER_API_KEY":                 "jup-secret",
		"PRIVY_APP_SECRET":                "privy-app-secret",
		"PRIVY_AUTHORIZATION_PRIVATE_KEY": "wallet-auth:privy-auth-secret",
		"PRIVY_WEBHOOK_SECRET":            "webhook-signing-secret",
		"SOLANA_RPC_URL":                  "https://rpc.example/rpc-secret",
		"RELAYER_PRIVATE_KEY":             "relayer-secret",
	}
	environ := make([]string, 0, 2+len(secrets))
	environ = append(environ, "MONACO_ENV=staging", "MONACO_TIMEOUT_JUPITER_EXECUTE=90s")
	for k, v := range secrets {
		environ = append(environ, k+"="+v)
	}
	cfg, err := config.Load(environ)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Redacted()
	tests := []struct {
		key  string
		want string
	}{
		{"DATABASE_URL", "***"},
		{"NATS_URL", "***"},
		{"OTEL_EXPORTER_OTLP_HEADERS", "***"},
		{"MONACO_DEV_TOKEN_KEY", "***"},
		{"JUPITER_API_KEY", "***"},
		{"MONACO_JUPITER_SWAP_BASE_URL", "https://api.jup.ag/swap/v2"},
		{"MONACO_JUPITER_PRICE_BASE_URL", "https://api.jup.ag/price/v3"},
		{"MONACO_ENV", "staging"},
		{"MONACO_HTTP_ADDR", ":8080"},
		{"MONACO_HTTP_MAX_BODY_BYTES", "1048576"},
		{"MONACO_WORKER_HEALTH_ADDR", ":8081"},
		{"MONACO_DB_MAX_CONNS", "10"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT", ""},
		{"OTEL_SERVICE_NAME", "monaco"},
		{"MONACO_TIMEOUT_RPC", "5s"},
		{"MONACO_TIMEOUT_PRIVY", "10s"},
		{"MONACO_TIMEOUT_JUPITER_QUOTE", "5s"},
		{"MONACO_TIMEOUT_JUPITER_EXECUTE", "1m30s"},
		{"MONACO_TIMEOUT_HTTP_SERVER_READ", "10s"},
		{"MONACO_TIMEOUT_HTTP_SERVER_WRITE", "30s"},
		{"MONACO_TIMEOUT_SHUTDOWN", "10s"},
		{"PRIVY_APP_SECRET", "***"},
		{"PRIVY_AUTHORIZATION_PRIVATE_KEY", "***"},
		{"PRIVY_WEBHOOK_SECRET", "***"},
		{"SOLANA_RPC_URL", "***"},
		{"RELAYER_PRIVATE_KEY", "***"},
		{"PRIVY_APP_ID", ""},
		{"PRIVY_VERIFICATION_KEY", ""},
		{"PRIVY_AUTHORIZATION_KEY_ID", ""},
		{"PRIVY_BASE_URL", "https://api.privy.io"},
		{"SOLANA_USDC_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"},
		{"MONACO_FAULTPOINT", ""},
	}
	if len(got) != len(tests) {
		t.Fatalf("Redacted has %d keys, want %d: %v", len(got), len(tests), got)
	}
	for _, tt := range tests {
		if got[tt.key] != tt.want {
			t.Errorf("Redacted()[%s] = %q, want %q", tt.key, got[tt.key], tt.want)
		}
	}
	for key, shown := range got {
		for _, secret := range secrets {
			if strings.Contains(shown, secret) {
				t.Errorf("Redacted()[%s] leaks a secret", key)
			}
		}
	}
}

func TestTestDBURLDefaultsToTheTestContainer(t *testing.T) {
	t.Parallel()
	if got := config.TestDBURL(required()); got != config.DefaultTestDBURL || !strings.Contains(got, ":54323/") {
		t.Fatalf("TestDBURL without TEST_DATABASE_URL = %q", got)
	}
	if got := config.TestDBURL(append(required(), "TEST_DATABASE_URL=")); got != config.DefaultTestDBURL {
		t.Fatalf("TestDBURL with an empty TEST_DATABASE_URL = %q", got)
	}
	custom := "postgres://ci@127.0.0.1:54323/ci"
	if got := config.TestDBURL(append(required(), "TEST_DATABASE_URL="+custom)); got != custom {
		t.Fatalf("TestDBURL = %q, want %q", got, custom)
	}
}
