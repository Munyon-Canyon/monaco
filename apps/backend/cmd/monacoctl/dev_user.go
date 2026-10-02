package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
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
)

func devTokenNewUser(cfg config.Config, ttl time.Duration, stdout, stderr io.Writer) int {
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		return devTokenFail(stderr, err)
	}
	ctx := context.Background()
	pool, conn, err := devStack(ctx, cfg)
	if err != nil {
		return devTokenFail(stderr, err)
	}
	defer pool.Close()
	defer conn.Close(ctx)
	user, err := devCreateUser(ctx, cfg, pool, conn)
	if err != nil {
		return devTokenFail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, verifier.Mint(user.UserID.String(), clk.Now().Add(ttl)))
	_, _ = fmt.Fprintf(stderr, "dev user %s @%s wallet %s\n", user.UserID, user.Handle, user.WalletAddress)
	return 0
}

func devStack(ctx context.Context, cfg config.Config) (*pgxpool.Pool, *bus.Conn, error) {
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return nil, nil, err
	}
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pool, conn, nil
}

func devCreateUser(
	ctx context.Context, cfg config.Config, pool *pgxpool.Pool, hints app.Hints,
) (app.DevUser, error) {
	clk := clock.Real{}
	client := chainprivy.New(cfg, clk)
	return app.CreateDevUser(ctx, app.CreateDevUserDeps{
		Env: cfg.Env, UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{},
		Privy: privyadapter.Users{Client: client}, Wallets: privyadapter.Wallets{Client: client},
		IDs: ids.Real{}, Clock: clk, Hints: hints, Rand: rand.Reader,
	})
}

func devTokenFail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "monacoctl dev token: %v\n", err)
	return 1
}
