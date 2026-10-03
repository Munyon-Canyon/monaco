package relayer_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const missingSig = chain.Signature(
	"5skwAZR52M2ujh65DRzE8YAxigoefp3JSsiJRt3raUiH5vaveUqt3wmFJPm5cko973ai1HkyodC6MdHXuCwZbpEb",
)

func TestRedaction_noPrivateKeyTokenOrSignatureReachesALogLine(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(
		t.Context(),
		observability.NewLogger(config.Config{Env: config.EnvProduction}, &logs),
	)
	s := overFakes(t, "relayer-at-floor")
	cfg := keyConfig("relayer-at-floor")
	cfg.Privy.WebhookSecret = "whsec_c2lnbmluZy1rZXktZm9yLXRlc3Rz"
	signed, err := s.transfers.Build(ctx, fund(5))
	if err != nil {
		t.Fatal(err)
	}
	expired := fakes.PrivyAccessToken(
		cfg.Privy.AppID,
		"did:privy:member",
		clock.Real{}.Now().Add(-2*time.Hour),
		time.Hour,
	)
	rpc := solana.New(cfg, clock.Real{}, httpclient.WithTransport(&recorder{handler: fakes.New()}))
	broken := cfg
	broken.Relayer.PrivateKey = cfg.Relayer.PrivateKey[:40]

	observability.Info(
		ctx,
		observability.BootConfig,
		slog.String("service", "test"),
		slog.Any("config", cfg.Redacted()),
	)
	_, errKey := relayer.New(broken, rpc)
	verifier, err := privy.New(cfg, clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	_, errToken := verifier.VerifyAccessToken(ctx, expired)
	_, errSig := rpc.InboundTransfers(ctx, missingSig, memberWallet)
	forged := signed
	forged.Signature = missingSig
	for _, err := range []error{
		errKey, errToken, errSig, s.relayer.CheckFloor(ctx), s.transfers.Broadcast(ctx, forged),
	} {
		if err == nil {
			t.Fatal("every step above must fail")
		}
		boundary.Stopped(ctx, "test", err)
	}
	out := string(logs.Bytes())
	for name, secret := range map[string]string{
		"relayer": cfg.Relayer.PrivateKey, "broken relayer": broken.Relayer.PrivateKey,
		"privy app": cfg.Privy.AppSecret, "authorization": cfg.Privy.AuthorizationPrivateKey[len("wallet-auth:"):],
		"webhook": "c2lnbmluZy1rZXktZm9yLXRlc3Rz", "access": expired,
		"transfer sig": string(signed.Signature), "looked-up sig": string(missingSig),
	} {
		if strings.Contains(out, secret) {
			t.Errorf("%s reached a log line:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "relayer_underfunded") || strings.Count(out, "\n") < 6 {
		t.Fatalf("expected the boot config and five failures in the log:\n%s", out)
	}
}
