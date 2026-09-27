package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func FuzzLoad(f *testing.F) {
	f.Fuzz(func(t *testing.T, overrides string) {
		cfg, err := config.Load(append(required(), strings.Split(overrides, "\n")...))
		if err != nil {
			if target := (*errs.Error)(nil); !errors.As(err, &target) || errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("Load error %v is not an *errs.Error with code invalid_input", err)
			}
			return
		}
		assertValid(t, cfg)
	})
}

func assertValid(t *testing.T, cfg config.Config) {
	t.Helper()
	timeouts := []time.Duration{
		cfg.Timeouts.RPC, cfg.Timeouts.Privy, cfg.Timeouts.JupiterQuote, cfg.Timeouts.JupiterExecute,
		cfg.Timeouts.HTTPServerRead, cfg.Timeouts.HTTPServerWrite, cfg.Timeouts.Shutdown,
	}
	for _, d := range timeouts {
		if d <= 0 {
			t.Fatalf("Load accepted a non-positive timeout: %+v", cfg.Timeouts)
		}
	}
	if cfg.DB.MaxConns <= 0 || cfg.DB.URL == "" || cfg.NATS.URL == "" {
		t.Fatalf("Load accepted an invalid config: %+v", cfg)
	}
	switch cfg.Env {
	case config.EnvLocal, config.EnvTest, config.EnvStaging, config.EnvProduction:
	default:
		t.Fatalf("Load accepted Env %q", cfg.Env)
	}
}
