package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func required() []string {
	return []string{
		"MONACO_ENV=local",
		"DATABASE_URL=postgres://monaco@localhost:54322/monaco",
		"NATS_URL=nats://localhost:4222",
	}
}

func apnsKeys() []string {
	return []string{"APNS_KEY_P8=p8-key", "APNS_KEY_ID=key-id", "APNS_TEAM_ID=team-id"}
}

func TestLoadFillsDefaultsFromTheRFC(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "PATH=/usr/bin", "HOME=/home/monaco"))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Env:  config.EnvLocal,
		HTTP: config.HTTP{Addr: ":8080", MaxBodyBytes: 1 << 20},
		Worker: config.Worker{
			HealthAddr: ":8081", FundSendWindow: 2 * time.Minute, WithdrawalUnsentAge: 2 * time.Minute,
			PollerBackoffMax: 5 * time.Minute,
		},
		DB:   config.DB{URL: "postgres://monaco@localhost:54322/monaco", MaxConns: 11},
		NATS: config.NATS{URL: "nats://localhost:4222"},
		Bus:  config.Bus{APIRelay: true},
		OTel: config.OTel{ServiceName: "monaco"},
		Timeouts: config.Timeouts{
			RPC:             5 * time.Second,
			RPCBreakerOpen:  time.Minute,
			Privy:           10 * time.Second,
			APNs:            10 * time.Second,
			JupiterQuote:    5 * time.Second,
			JupiterExecute:  2 * time.Minute,
			CoinGecko:       15 * time.Second,
			XStocks:         15 * time.Second,
			Tessera:         15 * time.Second,
			PreStocks:       15 * time.Second,
			PostHog:         3 * time.Second,
			Ably:            2 * time.Second,
			Storage:         10 * time.Second,
			HTTPServerRead:  10 * time.Second,
			HTTPServerWrite: 30 * time.Second,
			IdempotentRun:   25 * time.Second,
			Shutdown:        10 * time.Second,
		},
		Jupiter: config.Jupiter{
			SwapBaseURL:  "https://api.jup.ag/swap/v2",
			PriceBaseURL: "https://api.jup.ag/price/v3",
			RateLimit:    8,
		},
		CoinGecko: config.CoinGecko{
			Plan:          config.CoinGeckoDemo,
			BaseURL:       "https://api.coingecko.com/api/v3",
			MonthlyBudget: 10000,
		},
		XStocks:   config.XStocks{BaseURL: "https://api.xstocks.fi"},
		Tessera:   config.Tessera{BaseURL: "https://rest-api.tessera.pe"},
		PreStocks: config.PreStocks{BaseURL: "https://prestocks.com"},
		Market:    config.Market{PricePollInterval: 2 * time.Minute},
		Identity:  config.Identity{NudgesInterval: 24 * time.Hour, PhotoPurgesInterval: time.Minute},
		Ranking:   config.Ranking{CallsResolverInterval: time.Minute},
		Funding: config.Funding{
			DepositPollInterval: 30 * time.Second, DepositRPCRate: 20, DepositTickBudget: 480,
			DepositRotation: 6 * time.Hour, DepositRecoverySlots: 216000, DepositDiscovery: 6 * time.Hour,
			TreasuryReconcileInterval: 60 * time.Second,
			WindDownInterval:          2 * time.Minute,
			BounceSweepInterval:       30 * time.Second, BounceSweepAge: 2 * time.Minute,
			EVMDepositNetworks: []chain.Network{chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum},
			EVMForwardInterval: 10 * time.Second,
		},
		Privy:   config.Privy{BaseURL: "https://api.privy.io"},
		APNs:    config.APNs{Topic: "xyz.monacolabs.app"},
		PostHog: config.PostHog{Host: "https://us.i.posthog.com"},
		Ably:    config.Ably{RESTHost: "rest.ably.io"},
		Trade: config.Trade{
			Engine: config.TradeEngineLive, SwapSweepInterval: 30 * time.Second, SwapSweepAge: 2 * time.Minute,
		},
		Admin: config.Admin{DeadLettersInterval: 10 * time.Second, ApprovalsExpireInterval: 5 * time.Minute},
		Solana: config.Solana{
			RPCURL:   "https://api.mainnet-beta.solana.com",
			USDCMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		},
		EVM: config.EVM{
			RPCURLBase:      "https://mainnet.base.org",
			RPCURLArbitrum:  "https://arb1.arbitrum.io/rpc",
			RPCURLEthereum:  "https://ethereum-rpc.publicnode.com",
			RPCURLRobinhood: "https://rpc.mainnet.chain.robinhood.com",
			LogsMaxBlocks:   10,
		},
		Listings: config.Listings{
			OndoBaseURL: "https://app.ondo.finance", CoinbaseBaseURL: "https://api.coinbase.com",
			RobinhoodBaseURL: "https://api.robinhood.com",
		},
		Routing: config.Routing{
			UniswapBaseURL: "https://trade-api.gateway.uniswap.org", UniswapTimeout: 5 * time.Second,
			UniswapQuoteTimeout: 4 * time.Second,
			MinEdgeBps:          10, MaxImpactBps: 300,
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("Load = %+v, want %+v", cfg, want)
	}
}

func deployedMaxConns(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "render.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var blueprint struct {
		Services []struct {
			Name    string `yaml:"name"`
			EnvVars []struct {
				Key   string `yaml:"key"`
				Value string `yaml:"value"`
			} `yaml:"envVars"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &blueprint); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, svc := range blueprint.Services {
		for _, env := range svc.EnvVars {
			if env.Key == "MONACO_DB_MAX_CONNS" {
				got[svc.Name] = env.Value
			}
		}
	}
	return got
}

func TestDeployedPoolHoldsATransactionAndItsNestedRead(t *testing.T) {
	t.Parallel()
	const minConns = 2
	boot, err := os.ReadFile(filepath.Join("..", "..", "..", "cmd", "worker", "boot.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(boot), "locks, err := db.Open(ctx, config.DB{URL: d.Config.DB.URL, MaxConns: 1})") ||
		!strings.Contains(string(boot), "poller.NewRunner(locks,") {
		t.Fatal("worker pollers no longer lock on their own one-connection pool; " +
			"MONACO_DB_MAX_CONNS must then also cover every poller's held lock connection")
	}
	deployed := deployedMaxConns(t)
	for _, service := range []string{"monaco-api", "monaco-worker"} {
		value, ok := deployed[service]
		if !ok {
			t.Fatalf("render.yaml sets no MONACO_DB_MAX_CONNS for %s", service)
		}
		cfg, err := config.Load(append(required(), "MONACO_DB_MAX_CONNS="+value, "PATH=/usr/bin", "HOME=/home/monaco"))
		if err != nil {
			t.Fatalf("%s: loading the deployed MONACO_DB_MAX_CONNS=%s: %v", service, value, err)
		}
		if cfg.DB.MaxConns < minConns {
			t.Fatalf("%s deploys MONACO_DB_MAX_CONNS=%d, want at least %d: a transaction and its nested read",
				service, cfg.DB.MaxConns, minConns)
		}
	}
}

func TestLoadReadsEveryKey(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load([]string{
		"MONACO_ENV=production",
		"MONACO_HTTP_ADDR=127.0.0.1:9000",
		"MONACO_HTTP_MAX_BODY_BYTES=4096",
		"TRUST_PROXY_HEADERS=true",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:9001",
		"FUND_SEND_WINDOW=5s",
		"WITHDRAWAL_UNSENT_AGE=6s",
		"MONACO_POLLER_BACKOFF_MAX=7s",
		"DATABASE_URL=postgres://prod",
		"MONACO_DB_MAX_CONNS=40",
		"NATS_URL=nats://prod:4222",
		"OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.example",
		"OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic abc",
		"OTEL_SERVICE_NAME=monaco-api",
		"MONACO_DEV_TOKEN_KEY=dev-secret",
		"MONACO_TIMEOUT_RPC=1s",
		"MONACO_TIMEOUT_RPC_BREAKER_OPEN=3s",
		"MONACO_TIMEOUT_PRIVY=2s",
		"MONACO_TIMEOUT_APNS=8s",
		"MONACO_TIMEOUT_JUPITER_QUOTE=3s",
		"MONACO_TIMEOUT_JUPITER_EXECUTE=4m",
		"MONACO_TIMEOUT_COINGECKO=7s",
		"MONACO_TIMEOUT_XSTOCKS=8s",
		"MONACO_TIMEOUT_TESSERA=9s",
		"MONACO_TIMEOUT_PRESTOCKS=11s",
		"MONACO_TIMEOUT_POSTHOG=2500ms",
		"MONACO_TIMEOUT_ABLY=1500ms",
		"MONACO_TIMEOUT_HTTP_SERVER_READ=5s",
		"MONACO_TIMEOUT_HTTP_SERVER_WRITE=6s",
		"MONACO_TIMEOUT_IDEMPOTENT_RUN=3s",
		"MONACO_TIMEOUT_SHUTDOWN=7s",
		"MONACO_JUPITER_SWAP_BASE_URL=http://fakes/jupiter/swap/v2",
		"MONACO_JUPITER_PRICE_BASE_URL=http://fakes/jupiter/price/v3",
		"JUPITER_API_KEY=jup-secret",
		"MONACO_JUPITER_RATE_LIMIT=40",
		"COINGECKO_BASE_URL=http://fakes/coingecko",
		"COINGECKO_API_KEY=cg-secret",
		"MARKET_HISTORY_ENABLED=false",
		"COINGECKO_MONTHLY_BUDGET=4000",
		"COINGECKO_PLAN=pro",
		"XSTOCKS_BASE_URL=http://fakes/xstocks",
		"TESSERA_API_BASE_URL=http://fakes/tessera",
		"PRESTOCKS_API_BASE_URL=http://fakes/prestocks",
		"MARKET_PRICE_POLL_INTERVAL=1s",
		"IDENTITY_NUDGES_INTERVAL=2s",
		"IDENTITY_PHOTO_PURGES_INTERVAL=3s",
		"RANKING_CALLS_RESOLVER_INTERVAL=6s",
		"FUNDING_DEPOSIT_POLL_INTERVAL=2s",
		"FUNDING_DEPOSIT_RPC_RATE=12",
		"FUNDING_DEPOSIT_TICK_BUDGET=90",
		"FUNDING_DEPOSIT_ROTATION=7s",
		"FUNDING_DEPOSIT_RECOVERY_SLOTS=1234",
		"FUNDING_DEPOSIT_DISCOVERY=8s",
		"TRADE_SWAP_SWEEP_INTERVAL=4s",
		"TRADE_SWAP_SWEEP_AGE=5s",
		"ADMIN_DEADLETTERS_INTERVAL=8s",
		"ADMIN_APPROVALS_EXPIRE_INTERVAL=7s",
		"FUNDING_TREASURY_RECONCILE_INTERVAL=3s",
		"FUNDING_WINDDOWN_INTERVAL=9s",
		"FUNDING_BOUNCE_SWEEP_INTERVAL=6s",
		"FUNDING_BOUNCE_SWEEP_AGE=7s",
		"EVM_DEPOSIT_NETWORKS=arbitrum,base",
		"FUNDING_EVM_FORWARD_INTERVAL=4s",
		"PRIVY_APP_ID=app-id",
		"PRIVY_APP_SECRET=app-secret",
		"PRIVY_VERIFICATION_KEY=verification-pem",
		"PRIVY_AUTHORIZATION_PRIVATE_KEY=wallet-auth:key",
		"PRIVY_AUTHORIZATION_KEY_ID=quorum-id",
		"PRIVY_WEBHOOK_SECRET=whsec_x",
		"PRIVY_BASE_URL=http://fakes/privy",
		"SOLANA_RPC_URL=http://fakes/rpc",
		"SOLANA_USDC_MINT=mint",
		"EVM_RPC_URL_BASE=http://fakes/base",
		"EVM_RPC_URL_ARBITRUM=http://fakes/arbitrum",
		"EVM_RPC_URL_ETHEREUM=http://fakes/ethereum",
		"EVM_RPC_URL_ROBINHOOD=http://fakes/robinhood",
		"EVM_LOGS_MAX_BLOCKS=2000",
		"LISTINGS_SYNC_ENABLED=false",
		"ONDO_BASE_URL=http://fakes/ondo",
		"COINBASE_BASE_URL=http://fakes/coinbase",
		"ROBINHOOD_BASE_URL=http://fakes/robinhood-api",
		"UNISWAP_API_KEY=uni-secret",
		"UNISWAP_BASE_URL=http://fakes/uniswap",
		"MONACO_TIMEOUT_UNISWAP=3s",
		"MONACO_TIMEOUT_UNISWAP_QUOTE=2s",
		"ROUTER_MIN_EDGE_BPS=25",
		"ROUTER_MAX_IMPACT_BPS=150",
		"RELAYER_PRIVATE_KEY=relayer-key",
		"APNS_KEY_P8=p8-key",
		"APNS_KEY_ID=key-id",
		"APNS_TEAM_ID=team-id",
		"APNS_TOPIC=com.example.app",
		"POSTHOG_API_KEY=ph-secret",
		"POSTHOG_HOST=http://fakes/posthog",
		"ABLY_API_KEY=ably-secret",
		"ABLY_REST_HOST=http://fakes/ably",
		"SUPABASE_URL=http://127.0.0.1:54321",
		"SUPABASE_SERVICE_ROLE_KEY=sb-secret",
		"MONACO_TIMEOUT_STORAGE=4s",
		"MONACO_FAULTPOINT=before-commit",
		"MONACO_BUS_ACK_WAIT=100ms",
		"MONACO_BUS_API_RELAY=off",
		"FUND_PAGE_URL=https://fund.example/fund",
		"WEB_ALLOWED_ORIGINS=https://fund.example,http://127.0.0.1:5173",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Env:  config.EnvProduction,
		HTTP: config.HTTP{Addr: "127.0.0.1:9000", MaxBodyBytes: 4096, TrustProxyHeaders: true},
		Worker: config.Worker{
			HealthAddr: "127.0.0.1:9001", FundSendWindow: 5 * time.Second, WithdrawalUnsentAge: 6 * time.Second,
			PollerBackoffMax: 7 * time.Second,
		},
		DB:   config.DB{URL: "postgres://prod", MaxConns: 40},
		NATS: config.NATS{URL: "nats://prod:4222"},
		Bus:  config.Bus{AckWait: 100 * time.Millisecond},
		OTel: config.OTel{
			Endpoint:    "https://otlp.example",
			Headers:     "Authorization=Basic abc",
			ServiceName: "monaco-api",
		},
		Auth: config.Auth{DevTokenKey: "dev-secret"},
		Timeouts: config.Timeouts{
			RPC:             time.Second,
			RPCBreakerOpen:  3 * time.Second,
			Privy:           2 * time.Second,
			APNs:            8 * time.Second,
			JupiterQuote:    3 * time.Second,
			JupiterExecute:  4 * time.Minute,
			CoinGecko:       7 * time.Second,
			XStocks:         8 * time.Second,
			Tessera:         9 * time.Second,
			PreStocks:       11 * time.Second,
			PostHog:         2500 * time.Millisecond,
			Ably:            1500 * time.Millisecond,
			Storage:         4 * time.Second,
			HTTPServerRead:  5 * time.Second,
			HTTPServerWrite: 6 * time.Second,
			IdempotentRun:   3 * time.Second,
			Shutdown:        7 * time.Second,
		},
		Jupiter: config.Jupiter{
			SwapBaseURL:  "http://fakes/jupiter/swap/v2",
			PriceBaseURL: "http://fakes/jupiter/price/v3",
			APIKey:       "jup-secret",
			RateLimit:    40,
		},
		CoinGecko: config.CoinGecko{
			Plan:          config.CoinGeckoPro,
			BaseURL:       "http://fakes/coingecko",
			APIKey:        "cg-secret",
			MonthlyBudget: 4000,
		},
		XStocks:   config.XStocks{BaseURL: "http://fakes/xstocks"},
		Tessera:   config.Tessera{BaseURL: "http://fakes/tessera"},
		PreStocks: config.PreStocks{BaseURL: "http://fakes/prestocks"},
		Market:    config.Market{PricePollInterval: time.Second},
		Identity:  config.Identity{NudgesInterval: 2 * time.Second, PhotoPurgesInterval: 3 * time.Second},
		Ranking:   config.Ranking{CallsResolverInterval: 6 * time.Second},
		Funding: config.Funding{
			DepositPollInterval: 2 * time.Second, DepositRPCRate: 12, DepositTickBudget: 90,
			DepositRotation: 7 * time.Second, DepositRecoverySlots: 1234, DepositDiscovery: 8 * time.Second,
			TreasuryReconcileInterval: 3 * time.Second,
			WindDownInterval:          9 * time.Second,
			BounceSweepInterval:       6 * time.Second, BounceSweepAge: 7 * time.Second,
			EVMDepositNetworks: []chain.Network{chain.NetworkArbitrum, chain.NetworkBase},
			EVMForwardInterval: 4 * time.Second,
		},
		Privy: config.Privy{
			AppID: "app-id", AppSecret: "app-secret", VerificationKey: "verification-pem",
			AuthorizationPrivateKey: "wallet-auth:key", AuthorizationKeyID: "quorum-id",
			WebhookSecret: "whsec_x", BaseURL: "http://fakes/privy",
		},
		Solana: config.Solana{RPCURL: "http://fakes/rpc", USDCMint: "mint"},
		EVM: config.EVM{
			RPCURLBase: "http://fakes/base", RPCURLArbitrum: "http://fakes/arbitrum",
			RPCURLEthereum: "http://fakes/ethereum", RPCURLRobinhood: "http://fakes/robinhood",
			LogsMaxBlocks: 2000,
		},
		Listings: config.Listings{
			OndoBaseURL: "http://fakes/ondo", CoinbaseBaseURL: "http://fakes/coinbase",
			RobinhoodBaseURL: "http://fakes/robinhood-api",
		},
		Routing: config.Routing{
			UniswapAPIKey: "uni-secret", UniswapBaseURL: "http://fakes/uniswap", UniswapTimeout: 3 * time.Second,
			UniswapQuoteTimeout: 2 * time.Second,
			MinEdgeBps:          25, MaxImpactBps: 150,
		},
		Relayer:  config.Relayer{PrivateKey: "relayer-key"},
		APNs:     config.APNs{KeyP8: "p8-key", KeyID: "key-id", TeamID: "team-id", Topic: "com.example.app"},
		PostHog:  config.PostHog{APIKey: "ph-secret", Host: "http://fakes/posthog"},
		Ably:     config.Ably{APIKey: "ably-secret", RESTHost: "http://fakes/ably"},
		Supabase: config.Supabase{URL: "http://127.0.0.1:54321", ServiceRoleKey: "sb-secret"},
		Web: config.Web{
			FundPageURL: "https://fund.example/fund", AllowedOrigins: "https://fund.example,http://127.0.0.1:5173",
		},
		Trade: config.Trade{
			Engine: config.TradeEngineLive, SwapSweepInterval: 4 * time.Second, SwapSweepAge: 5 * time.Second,
		},
		Admin:      config.Admin{DeadLettersInterval: 8 * time.Second, ApprovalsExpireInterval: 7 * time.Second},
		Faultpoint: "before-commit",
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadIgnoresIOSOnlyKeys(t *testing.T) {
	t.Parallel()
	environ := append(required(),
		"MONACO_STAGING_API_BASE_URL=https://staging.example.com",
		"MONACO_PRODUCTION_API_BASE_URL=https://api.example.com",
	)
	if _, err := config.Load(environ); err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err := config.Load(append(environ, "MONACO_FOO=1"))
	if err == nil || !strings.Contains(err.Error(), "unknown MONACO_FOO") {
		t.Fatalf("Load with MONACO_FOO = %v, want unknown MONACO_FOO", err)
	}
}

func TestLoadAcceptsEveryEnv(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging, config.EnvProduction} {
		environ := append(append(required(), apnsKeys()...), "MONACO_ENV="+string(env), "ABLY_API_KEY=ably-key")
		if env == config.EnvProduction {
			environ = append(environ, "POSTHOG_API_KEY=ph-key")
		}
		cfg, err := config.Load(environ)
		if err != nil {
			t.Fatalf("MONACO_ENV=%s: %v", env, err)
		}
		if cfg.Env != env {
			t.Fatalf("Env = %q, want %q", cfg.Env, env)
		}
	}
}

func TestLoadBootsWithoutACoinGeckoKeyInEveryEnv(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging, config.EnvProduction} {
		environ := append(append(required(), apnsKeys()...), "MONACO_ENV="+string(env), "POSTHOG_API_KEY=ph-key",
			"ABLY_API_KEY=ably-key")
		cfg, err := config.Load(environ)
		if err != nil || cfg.CoinGecko.APIKey != "" {
			t.Fatalf("MONACO_ENV=%s: key %q, err %v; want no key and no error", env, cfg.CoinGecko.APIKey, err)
		}
	}
}

func TestEnvDeployedIsStagingAndProductionOnly(t *testing.T) {
	t.Parallel()
	for env, want := range map[config.Env]bool{
		config.EnvLocal:      false,
		config.EnvTest:       false,
		config.EnvStaging:    true,
		config.EnvProduction: true,
		"":                   false,
	} {
		if got := env.Deployed(); got != want {
			t.Errorf("Env(%q).Deployed() = %v, want %v", env, got, want)
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
			name:    "bus knobs",
			environ: append(required(), "MONACO_BUS_ACK_WAIT=0s", "MONACO_BUS_API_RELAY=maybe"),
			want: "config.Load: invalid_input: invalid MONACO_BUS_ACK_WAIT (empty or a positive duration like 100ms), " +
				"MONACO_BUS_API_RELAY (on or off)",
		},
		{
			name:    "trade engine outside its two values",
			environ: append(required(), "TRADE_ENGINE=paper"),
			want:    "config.Load: invalid_input: invalid TRADE_ENGINE (live or stub)",
		},
		{
			name:    "stub trade engine outside local dev",
			environ: append(required(), "MONACO_ENV=test", "TRADE_ENGINE=stub"),
			want:    "config.Load: invalid_input: invalid TRADE_ENGINE (stub runs only in local dev)",
		},
		{
			name: "production with a partial APNs key",
			environ: append(
				required(),
				"MONACO_ENV=production",
				"POSTHOG_API_KEY=ph-key",
				"ABLY_API_KEY=ably-key",
				"APNS_KEY_ID=key-id",
			),
			want: "config.Load: invalid_input: missing APNS_KEY_P8, APNS_TEAM_ID",
		},
		{
			name:    "a local APNs key needs its ids",
			environ: append(required(), "APNS_KEY_P8=p8-key"),
			want:    "config.Load: invalid_input: missing APNS_KEY_ID, APNS_TEAM_ID",
		},
		{
			name: "production refuses an APNs base URL",
			environ: append(
				append(required(), apnsKeys()...),
				"MONACO_ENV=production",
				"POSTHOG_API_KEY=ph-key",
				"ABLY_API_KEY=ably-key",
				"APNS_BASE_URL=http://fakes/apns",
			),
			want: "config.Load: invalid_input: invalid APNS_BASE_URL (not allowed in production)",
		},
		{
			name: "missing keys and a production base URL in one error",
			environ: append(
				required(),
				"MONACO_ENV=production",
				"POSTHOG_API_KEY=ph-key",
				"ABLY_API_KEY=ably-key",
				"APNS_BASE_URL=http://fakes/apns",
			),
			want: "config.Load: invalid_input: missing APNS_KEY_P8, APNS_KEY_ID, APNS_TEAM_ID; " +
				"invalid APNS_BASE_URL (not allowed in production)",
		},
		{
			name:    "APNs timeout that is not a positive duration",
			environ: append(required(), "MONACO_TIMEOUT_APNS=0s"),
			want:    "config.Load: invalid_input: invalid MONACO_TIMEOUT_APNS (positive duration like 5s)",
		},
		{
			name:    "production without a PostHog key",
			environ: append(append(required(), apnsKeys()...), "MONACO_ENV=production", "ABLY_API_KEY=ably-key"),
			want:    "config.Load: invalid_input: missing POSTHOG_API_KEY",
		},
		{
			name: "an empty PostHog key counts as missing in production",
			environ: append(append(required(), apnsKeys()...), "MONACO_ENV=production", "POSTHOG_API_KEY=",
				"ABLY_API_KEY=ably-key"),
			want: "config.Load: invalid_input: missing POSTHOG_API_KEY",
		},
		{
			name:    "production names every missing key in one error",
			environ: []string{"MONACO_ENV=production", "NATS_URL=nats://x"},
			want: "config.Load: invalid_input: missing DATABASE_URL, APNS_KEY_P8, APNS_KEY_ID, APNS_TEAM_ID, " +
				"POSTHOG_API_KEY, ABLY_API_KEY",
		},
		{
			name:    "trust proxy headers not a boolean",
			environ: append(required(), "TRUST_PROXY_HEADERS=render"),
			want:    "config.Load: invalid_input: invalid TRUST_PROXY_HEADERS (true or false)",
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
		"COINGECKO_API_KEY":               "cg-secret",
		"PRIVY_APP_SECRET":                "privy-app-secret",
		"PRIVY_AUTHORIZATION_PRIVATE_KEY": "wallet-auth:privy-auth-secret",
		"PRIVY_WEBHOOK_SECRET":            "webhook-signing-secret",
		"SOLANA_RPC_URL":                  "https://rpc.example/rpc-secret",
		"RELAYER_PRIVATE_KEY":             "relayer-secret",
		"APNS_KEY_P8":                     "p8-secret",
		"POSTHOG_API_KEY":                 "ph-api-secret",
		"ABLY_API_KEY":                    "ably-api-secret",
		"SUPABASE_SERVICE_ROLE_KEY":       "supabase-service-secret",
	}
	environ := make([]string, 0, 2+len(secrets))
	environ = append(environ, "MONACO_ENV=staging", "MONACO_TIMEOUT_JUPITER_EXECUTE=90s",
		"APNS_KEY_ID=key-id", "APNS_TEAM_ID=team-id", "NATS_CREDS=/run/secrets/nats.creds")
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
		{"NATS_CREDS", "***"},
		{"OTEL_EXPORTER_OTLP_HEADERS", "***"},
		{"MONACO_DEV_TOKEN_KEY", "***"},
		{"JUPITER_API_KEY", "***"},
		{"COINGECKO_API_KEY", "***"},
		{"COINGECKO_BASE_URL", "https://api.coingecko.com/api/v3"},
		{"MARKET_HISTORY_ENABLED", "true"},
		{"COINGECKO_MONTHLY_BUDGET", "10000"},
		{"COINGECKO_PLAN", "demo"},
		{"MONACO_TIMEOUT_COINGECKO", "15s"},
		{"MONACO_JUPITER_SWAP_BASE_URL", "https://api.jup.ag/swap/v2"},
		{"MONACO_JUPITER_PRICE_BASE_URL", "https://api.jup.ag/price/v3"},
		{"MONACO_JUPITER_RATE_LIMIT", "8"},
		{"MONACO_ENV", "staging"},
		{"MONACO_HTTP_ADDR", ":8080"},
		{"MONACO_HTTP_MAX_BODY_BYTES", "1048576"},
		{"MONACO_WORKER_HEALTH_ADDR", ":8081"},
		{"FUND_SEND_WINDOW", "2m0s"},
		{"WITHDRAWAL_UNSENT_AGE", "2m0s"},
		{"MONACO_POLLER_BACKOFF_MAX", "5m0s"},
		{"MONACO_DB_MAX_CONNS", "11"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT", ""},
		{"OTEL_SERVICE_NAME", "monaco"},
		{"MONACO_TIMEOUT_RPC", "5s"},
		{"MONACO_TIMEOUT_RPC_BREAKER_OPEN", "1m0s"},
		{"MONACO_TIMEOUT_PRIVY", "10s"},
		{"MONACO_TIMEOUT_JUPITER_QUOTE", "5s"},
		{"MONACO_TIMEOUT_JUPITER_EXECUTE", "1m30s"},
		{"MONACO_TIMEOUT_XSTOCKS", "15s"},
		{"XSTOCKS_BASE_URL", "https://api.xstocks.fi"},
		{"MONACO_TIMEOUT_TESSERA", "15s"},
		{"TESSERA_API_BASE_URL", "https://rest-api.tessera.pe"},
		{"MONACO_TIMEOUT_PRESTOCKS", "15s"},
		{"PRESTOCKS_API_BASE_URL", "https://prestocks.com"},
		{"MARKET_PRICE_POLL_INTERVAL", "2m0s"},
		{"IDENTITY_NUDGES_INTERVAL", "24h0m0s"},
		{"IDENTITY_PHOTO_PURGES_INTERVAL", "1m0s"},
		{"RANKING_CALLS_RESOLVER_INTERVAL", "1m0s"},
		{"FUNDING_DEPOSIT_POLL_INTERVAL", "30s"},
		{"FUNDING_DEPOSIT_RPC_RATE", "20"},
		{"FUNDING_DEPOSIT_TICK_BUDGET", "480"},
		{"FUNDING_DEPOSIT_ROTATION", "6h0m0s"},
		{"FUNDING_DEPOSIT_RECOVERY_SLOTS", "216000"},
		{"FUNDING_DEPOSIT_DISCOVERY", "6h0m0s"},
		{"FUNDING_TREASURY_RECONCILE_INTERVAL", "1m0s"},
		{"FUNDING_WINDDOWN_INTERVAL", "2m0s"},
		{"FUNDING_BOUNCE_SWEEP_INTERVAL", "30s"},
		{"FUNDING_BOUNCE_SWEEP_AGE", "2m0s"},
		{"EVM_DEPOSIT_NETWORKS", "base,arbitrum,ethereum"},
		{"FUNDING_EVM_FORWARD_INTERVAL", "10s"},
		{"MONACO_TIMEOUT_HTTP_SERVER_READ", "10s"},
		{"MONACO_TIMEOUT_HTTP_SERVER_WRITE", "30s"},
		{"MONACO_TIMEOUT_IDEMPOTENT_RUN", "25s"},
		{"MONACO_TIMEOUT_SHUTDOWN", "10s"},
		{"PRIVY_APP_SECRET", "***"},
		{"PRIVY_AUTHORIZATION_PRIVATE_KEY", "***"},
		{"PRIVY_WEBHOOK_SECRET", "***"},
		{"SOLANA_RPC_URL", "***"},
		{"RELAYER_PRIVATE_KEY", "***"},
		{"APNS_KEY_P8", "***"},
		{"APNS_KEY_ID", "key-id"},
		{"APNS_TEAM_ID", "team-id"},
		{"APNS_TOPIC", "xyz.monacolabs.app"},
		{"APNS_BASE_URL", ""},
		{"MONACO_TIMEOUT_APNS", "10s"},
		{"POSTHOG_API_KEY", "***"},
		{"POSTHOG_HOST", "https://us.i.posthog.com"},
		{"MONACO_TIMEOUT_POSTHOG", "3s"},
		{"ABLY_API_KEY", "***"},
		{"ABLY_REST_HOST", "rest.ably.io"},
		{"MONACO_TIMEOUT_ABLY", "2s"},
		{"SUPABASE_URL", ""},
		{"SUPABASE_SERVICE_ROLE_KEY", "***"},
		{"MONACO_TIMEOUT_STORAGE", "10s"},
		{"PRIVY_APP_ID", ""},
		{"PRIVY_VERIFICATION_KEY", ""},
		{"PRIVY_AUTHORIZATION_KEY_ID", ""},
		{"PRIVY_BASE_URL", "https://api.privy.io"},
		{"SOLANA_USDC_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"},
		{"MONACO_FAULTPOINT", ""},
		{"MONACO_BUS_ACK_WAIT", ""},
		{"MONACO_BUS_API_RELAY", "on"},
		{"TRADE_ENGINE", "live"},
		{"TRADE_SWAP_SWEEP_INTERVAL", "30s"},
		{"TRADE_SWAP_SWEEP_AGE", "2m0s"},
		{"ADMIN_DEADLETTERS_INTERVAL", "10s"},
		{"ADMIN_APPROVALS_EXPIRE_INTERVAL", "5m0s"},
		{"TRUST_PROXY_HEADERS", "false"},
		{"FUND_PAGE_URL", ""},
		{"WEB_ALLOWED_ORIGINS", ""},
		{"EVM_RPC_URL_BASE", "***"},
		{"EVM_RPC_URL_ARBITRUM", "***"},
		{"EVM_RPC_URL_ETHEREUM", "***"},
		{"EVM_RPC_URL_ROBINHOOD", "***"},
		{"EVM_LOGS_MAX_BLOCKS", "10"},
		{"LISTINGS_SYNC_ENABLED", "true"},
		{"ONDO_BASE_URL", "https://app.ondo.finance"},
		{"COINBASE_BASE_URL", "https://api.coinbase.com"},
		{"ROBINHOOD_BASE_URL", "https://api.robinhood.com"},
		{"UNISWAP_API_KEY", "***"},
		{"UNISWAP_BASE_URL", "https://trade-api.gateway.uniswap.org"},
		{"MONACO_TIMEOUT_UNISWAP", "5s"},
		{"MONACO_TIMEOUT_UNISWAP_QUOTE", "4s"},
		{"ROUTER_MIN_EDGE_BPS", "10"},
		{"ROUTER_MAX_IMPACT_BPS", "300"},
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

func TestLoad_busKnobsRoundTripThroughRedacted(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "MONACO_BUS_ACK_WAIT=100ms", "MONACO_BUS_API_RELAY=off"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Redacted(); got["MONACO_BUS_ACK_WAIT"] != "100ms" || got["MONACO_BUS_API_RELAY"] != "off" {
		t.Fatalf("Redacted = %v, want the ack wait and relay switch as set", got)
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

func TestEVMRPCURLNamesEachNetworkAndNoneForSolana(t *testing.T) {
	t.Parallel()
	e := config.EVM{RPCURLBase: "b", RPCURLArbitrum: "a", RPCURLEthereum: "e", RPCURLRobinhood: "r"}
	for n, want := range map[chain.Network]string{
		chain.NetworkBase: "b", chain.NetworkArbitrum: "a", chain.NetworkEthereum: "e",
		chain.NetworkRobinhood: "r", chain.NetworkSolana: "",
	} {
		if got := e.RPCURL(n); got != want {
			t.Errorf("RPCURL(%s) = %q, want %q", n, got, want)
		}
	}
}

func TestLoadCoinGeckoPlanPicksTheHostHeaderAndBudget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		extra   []string
		plan    config.CoinGeckoPlan
		baseURL string
		header  string
		budget  int32
	}{
		{"demo", nil, config.CoinGeckoDemo, "https://api.coingecko.com/api/v3", "x-cg-demo-api-key", 10000},
		{
			"pro",
			[]string{"COINGECKO_PLAN=pro"},
			config.CoinGeckoPro,
			"https://pro-api.coingecko.com/api/v3", "x-cg-pro-api-key", 0,
		},
		{
			"zero turns the guard off",
			[]string{"COINGECKO_MONTHLY_BUDGET=0"},
			config.CoinGeckoDemo,
			"https://api.coingecko.com/api/v3", "x-cg-demo-api-key", 0,
		},
		{
			"pro with a budget and a host",
			[]string{"COINGECKO_PLAN=pro", "COINGECKO_MONTHLY_BUDGET=500", "COINGECKO_BASE_URL=http://x"},
			config.CoinGeckoPro, "http://x", "x-cg-pro-api-key", 500,
		},
	} {
		cfg, err := config.Load(append(required(), tc.extra...))
		cg := cfg.CoinGecko
		if err != nil || cg.Plan != tc.plan || cg.BaseURL != tc.baseURL || cg.Plan.KeyHeader() != tc.header ||
			cg.MonthlyBudget != tc.budget {
			t.Fatalf("%s: %+v, %v, want %s %s %s %d", tc.name, cg, err, tc.plan, tc.baseURL, tc.header, tc.budget)
		}
	}
	for _, bad := range []string{"COINGECKO_PLAN=free", "COINGECKO_MONTHLY_BUDGET=-1"} {
		if _, err := config.Load(append(required(), bad)); err == nil {
			t.Fatalf("%s loaded, want an error", bad)
		}
	}
}
