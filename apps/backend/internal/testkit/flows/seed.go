package flows

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
	seeds := map[string]Seeder{}
	each("Seeds", func(_ string, out any) { maps.Copy(seeds, out.(map[string]Seeder)) })
	return seeds
}

const seedTokenTTL = time.Hour

func seedConfig(env SeedEnv) (config.Config, error) {
	return config.Load([]string{
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
}

func seedSignedIn(ctx context.Context, env SeedEnv) (SeedResult, error) {
	cfg, err := seedConfig(env)
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

var errSeed = errors.New("flows: seed")

type seedFailure struct{ err error }

type seederT struct{ ctx func() context.Context }

func (seederT) Helper() {}

func (t seederT) Context() context.Context { return t.ctx() }

func (seederT) Fatalf(format string, args ...any) {
	panic(seedFailure{fmt.Errorf("%w: %s", errSeed, fmt.Sprintf(format, args...))})
}

func seedSignedInWith(arrange func(t testkit.SeedT, pool *pgxpool.Pool, user ids.UserID) map[string]string) Seeder {
	return func(ctx context.Context, env SeedEnv) (result SeedResult, err error) {
		result, err = seedSignedIn(ctx, env)
		if err != nil {
			return SeedResult{}, err
		}
		user, err := ids.ParseUserID(result.UserID)
		if err != nil {
			return SeedResult{}, err
		}
		cfg, err := seedConfig(env)
		if err != nil {
			return SeedResult{}, err
		}
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return SeedResult{}, err
		}
		defer pool.Close()
		defer func() {
			if r := recover(); r != nil {
				failure, ok := r.(seedFailure)
				if !ok {
					panic(r)
				}
				result, err = SeedResult{}, failure.err
			}
		}()
		maps.Copy(result.IDs, arrange(seederT{func() context.Context { return ctx }}, pool, user))
		return result, nil
	}
}
