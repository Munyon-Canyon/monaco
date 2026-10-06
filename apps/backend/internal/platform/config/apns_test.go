package config_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestLoad_apnsKeyIsOptionalInLocalAndTest(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest} {
		cfg, err := config.Load(append(required(), "MONACO_ENV="+string(env)))
		if err != nil {
			t.Fatalf("MONACO_ENV=%s without an APNs key: %v", env, err)
		}
		if want := (config.APNs{Topic: "com.monaco.app"}); cfg.APNs != want {
			t.Fatalf("MONACO_ENV=%s APNs = %+v, want %+v", env, cfg.APNs, want)
		}
	}
}

func TestLoad_apnsBaseURLIsAllowedOutsideProduction(t *testing.T) {
	t.Parallel()
	const base = "http://127.0.0.1:8099/apns"
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging} {
		environ := append(append(required(), apnsKeys()...), "MONACO_ENV="+string(env), "APNS_BASE_URL="+base,
			"ABLY_API_KEY=ably-key", agentKeyEnv)
		cfg, err := config.Load(environ)
		if err != nil {
			t.Fatalf("MONACO_ENV=%s with APNS_BASE_URL: %v", env, err)
		}
		if cfg.APNs.BaseURL != base {
			t.Fatalf("MONACO_ENV=%s BaseURL = %q, want %q", env, cfg.APNs.BaseURL, base)
		}
	}
}

func TestLoad_anEmptyAPNsTopicFallsBackToTheBundleID(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "APNS_TOPIC="))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APNs.Topic != "com.monaco.app" {
		t.Fatalf("Topic = %q, want com.monaco.app", cfg.APNs.Topic)
	}
}
