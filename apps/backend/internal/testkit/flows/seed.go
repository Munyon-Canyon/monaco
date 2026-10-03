package flows

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type SeedEnv struct{ DatabaseURL, NatsURL, FakesURL, TokenKey string }

type SeedResult struct {
	Token  string            `json:"token"`
	UserID string            `json:"user_id"`
	IDs    map[string]string `json:"ids"`
}

type Seeder func(ctx context.Context, env SeedEnv) (SeedResult, error)

func Seeds() map[string]Seeder {
	return map[string]Seeder{
		"F00RecordPingOK":                seedSignedIn,
		"F00RecordPingInvalidInput":      seedSignedIn,
		"F00RecordPingUnauthorized":      seedAnonymous,
		"F00RecordPingCrashAfterPublish": seedSignedIn,
	}
}

const seedTokenTTL = time.Hour

func seedSignedIn(ctx context.Context, env SeedEnv) (SeedResult, error) {
	cfg, err := config.Load([]string{
		"MONACO_ENV=test",
		"DATABASE_URL=" + env.DatabaseURL,
		"NATS_URL=" + env.NatsURL,
		"MONACO_DEV_TOKEN_KEY=" + env.TokenKey,
		"PRIVY_BASE_URL=" + env.FakesURL + "/privy",
		"PRIVY_APP_ID=seed-app",
		"PRIVY_APP_SECRET=seed-app-secret",
		"PRIVY_VERIFICATION_KEY=" + fakes.PrivyVerificationKey(),
		"PRIVY_AUTHORIZATION_KEY_ID=" + fakes.PrivyAuthorizationKeyID,
		"PRIVY_AUTHORIZATION_PRIVATE_KEY=" + fakes.PrivyAuthorizationKeyConfig(),
	})
	if err != nil {
		return SeedResult{}, err
	}
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		return SeedResult{}, err
	}
	user, err := NewDevUser(ctx, cfg)
	if err != nil {
		return SeedResult{}, err
	}
	return SeedResult{
		Token:  verifier.Mint(user.UserID.String(), clk.Now().Add(seedTokenTTL)),
		UserID: user.UserID.String(),
		IDs:    map[string]string{},
	}, nil
}

func seedAnonymous(context.Context, SeedEnv) (SeedResult, error) {
	return SeedResult{IDs: map[string]string{}}, nil
}

func NewDevUser(ctx context.Context, cfg config.Config) (app.DevUser, error) {
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return app.DevUser{}, err
	}
	defer pool.Close()
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
	if err != nil {
		return app.DevUser{}, err
	}
	defer conn.Close(ctx)
	clk := clock.Real{}
	client, err := chainprivy.New(cfg, clk)
	if err != nil {
		return app.DevUser{}, err
	}
	return app.CreateDevUser(ctx, app.CreateDevUserDeps{
		Env: cfg.Env, UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{},
		Privy: privyadapter.Users{Client: client}, Wallets: privyadapter.Wallets{Client: client},
		IDs: ids.Real{}, Clock: clk, Hints: conn, Rand: rand.Reader,
	})
}
