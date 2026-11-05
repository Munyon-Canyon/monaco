package config_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func deployed(env string, extra ...string) []string {
	base := append(required(), apnsKeys()...)
	base = append(base, "POSTHOG_API_KEY=ph-key", "ABLY_API_KEY=ably-key", "MONACO_ENV="+env)
	return append(base, extra...)
}

func TestLoad_productionWithTheRealPrivyAPIAndNoWalletURLBoots(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"staging", "production"} {
		cfg, err := config.Load(deployed(env))
		if err != nil {
			t.Fatalf("MONACO_ENV=%s with the default Privy URLs: %v", env, err)
		}
		if cfg.Privy.WalletURL() != "https://api.privy.io" {
			t.Fatalf("MONACO_ENV=%s WalletURL() = %q, want the real Privy API", env, cfg.Privy.WalletURL())
		}
	}
}

func TestLoad_aDeployedEnvRefusesAPrivyWalletBaseURL(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"staging", "production"} {
		_, err := config.Load(deployed(env, "PRIVY_WALLET_BASE_URL=https://api.privy.io"))
		if err == nil ||
			!strings.Contains(err.Error(), "PRIVY_WALLET_BASE_URL (not allowed in staging or production)") {
			t.Fatalf("MONACO_ENV=%s with PRIVY_WALLET_BASE_URL: err = %v, want a refusal", env, err)
		}
	}
}

func TestLoad_aDeployedEnvRefusesLiveTradingOnANonPrivyAPI(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"staging", "production"} {
		_, err := config.Load(deployed(env, "PRIVY_BASE_URL=http://fakes/privy"))
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"PRIVY_BASE_URL (live trading in staging or production needs the real Privy API)",
			) {
			t.Fatalf("MONACO_ENV=%s live trading on a fake Privy: err = %v, want a refusal", env, err)
		}
	}
}

func TestLoad_localAllowsAPrivyWalletBaseURLWithLiveTrading(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "PRIVY_WALLET_BASE_URL=http://fakes/privy", "TRADE_ENGINE=live"))
	if err != nil {
		t.Fatalf("local with PRIVY_WALLET_BASE_URL: %v", err)
	}
	if cfg.Privy.WalletBaseURL != "http://fakes/privy" || cfg.Privy.WalletURL() != "http://fakes/privy" {
		t.Fatalf("Privy = %+v, want the wallet base URL read", cfg.Privy)
	}
}
