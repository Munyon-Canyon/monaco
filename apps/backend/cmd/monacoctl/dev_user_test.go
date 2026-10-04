package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestDevToken_userNew(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cfg := devNewUserConfig(t, pool, nil)
	token, id, handle := runDevTokenNew(t, cfg)
	assertDevTokenVerifies(t, cfg, token, id)
	assertDevMe(t, pool, cfg, token, handle)
}

func TestDevToken_userNewRefusesBeforeItMints(t *testing.T) {
	t.Parallel()
	t.Run("database", func(t *testing.T) {
		t.Parallel()
		assertDevTokenRefused(t, devConfig(config.EnvLocal), "monacoctl dev token: db.Open:")
	})
	t.Run("nats", func(t *testing.T) {
		t.Parallel()
		cfg := devNewUserConfig(t, testkit.DB(t), nil)
		cfg.NATS.URL = "nats://127.0.0.1:1"
		assertDevTokenRefused(t, cfg, "monacoctl dev token: bus.Connect:")
	})
	t.Run("production", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		hits := 0
		cfg := devNewUserConfig(t, pool, &hits)
		cfg.Env = config.EnvProduction
		assertDevTokenRefused(t, cfg, devVerifierRefused)
		assertNoDevRows(t, pool, hits)
	})
	t.Run("no key", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		hits := 0
		cfg := devNewUserConfig(t, pool, &hits)
		cfg.Auth.DevTokenKey = ""
		assertDevTokenRefused(t, cfg, devVerifierRefused)
		assertNoDevRows(t, pool, hits)
	})
	t.Run("privy key", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		hits := 0
		cfg := devNewUserConfig(t, pool, &hits)
		cfg.Privy.VerificationKey = ""
		assertDevTokenRefused(t, cfg, "monacoctl dev token: privy.New: invalid_input")
		assertNoDevRows(t, pool, hits)
	})
	t.Run("privy", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		cfg := devNewUserConfig(t, pool, nil)
		down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		t.Cleanup(down.Close)
		cfg.Privy.BaseURL = down.URL + "/privy"
		assertDevTokenRefused(t, cfg, "monacoctl dev token: privy.CreateUser: invalid_input\n")
		assertNoDevRows(t, pool, 0)
	})
}

func runDevTokenNew(t *testing.T, cfg config.Config) (token, id, handle string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"token", "--user", "new"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	token = strings.TrimSuffix(stdout.String(), "\n")
	if strings.Contains(token, "\n") {
		t.Fatalf("stdout = %q, want the token alone", stdout.String())
	}
	var wallet string
	if _, err := fmt.Sscanf(stderr.String(), "dev user %s @%s wallet %s\n", &id, &handle, &wallet); err != nil {
		t.Fatalf("stderr %q: %v", stderr.String(), err)
	}
	if _, err := ids.ParseUserID(id); err != nil || !strings.HasPrefix(handle, "dev_") || len(handle) != 12 ||
		wallet == "" {
		t.Fatalf("id %s handle %s wallet %s", id, handle, wallet)
	}
	return token, id, handle
}

func assertDevTokenVerifies(t *testing.T, cfg config.Config, token, id string) {
	t.Helper()
	verifier, err := auth.NewDevVerifier(cfg, clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := verifier.Verify(t.Context(), token)
	if err != nil || actor != (auth.Actor{Kind: auth.ActorUser, ID: id, Standing: auth.StandingActive}) {
		t.Fatalf("Verify = %+v, %v", actor, err)
	}
}

func assertDevMe(t *testing.T, pool *pgxpool.Pool, cfg config.Config, token, handle string) {
	t.Helper()
	status, me := devMe(t, pool, cfg, token)
	if status != http.StatusOK || me.Handle != handle || me.Wallet == "" {
		t.Fatalf("GET /v1/me = %d %+v", status, me)
	}
}

func assertNoDevRows(t *testing.T, pool *pgxpool.Pool, hits int) {
	t.Helper()
	var users, wallets int
	err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM user_wallets)`).Scan(&users, &wallets)
	if err != nil || users != 0 || wallets != 0 || hits != 0 {
		t.Fatalf("users %d wallets %d privy requests %d, %v", users, wallets, hits, err)
	}
}

func assertDevTokenRefused(t *testing.T, cfg config.Config, want string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := devCmd(cfg, []string{"token", "--user", "new"}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), want) {
		t.Fatalf("exit %d stdout %q stderr %q, want prefix %q", code, stdout.String(), stderr.String(), want)
	}
}

func devNewUserConfig(t *testing.T, pool *pgxpool.Pool, hits *int) config.Config {
	t.Helper()
	fake := fakes.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	cfg := devConfig(config.EnvLocal)
	cfg.DB.URL = pool.Config().ConnString()
	cfg.DB.MaxConns = 4
	cfg.NATS.URL = testkit.NATSURL()
	cfg.Privy.BaseURL = srv.URL + "/privy"
	cfg.Privy.AppID = "app"
	cfg.Privy.AppSecret = "secret"
	cfg.Privy.AuthorizationKeyID = fakes.PrivyAuthorizationKeyID
	cfg.Privy.VerificationKey = fakes.PrivyVerificationKey()
	cfg.Timeouts.Privy = 5 * time.Second
	return cfg
}

type devMeBody struct {
	Handle string `json:"handle"`
	Wallet string `json:"member_wallet_address"`
}

func devMe(t *testing.T, pool *pgxpool.Pool, cfg config.Config, token string) (int, devMeBody) {
	t.Helper()
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		t.Fatal(err)
	}
	mount := identity.New(
		module.Deps{Pool: pool, UoW: db.New(pool, ids.Real{}, clk), IDs: ids.Real{}, Clock: clk},
		identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{}),
	).Mount
	handler, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          ids.Real{},
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var me devMeBody
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return rec.Code, me
}
