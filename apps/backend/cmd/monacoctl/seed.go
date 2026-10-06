package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const seedUsage = "usage: monacoctl seed <scenario> --users <id>,<id>"

func toolSeed(env toolEnv) tool { return seedTool(env.environ) }

func seedTool(environ []string) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		cfg, err := config.Load(environ)
		if err != nil {
			return fail(stderr, err)
		}
		return seedOn(cfg, db.Open, args, stdout, stderr)
	}
}

func seedDurables() []string {
	return []string{"treasury_trades", "ranking_membership", "ranking_triggers"}
}

func seedOn(cfg config.Config, open openDB, args []string, stdout, stderr io.Writer) int {
	name, users, ok := parseSeedArgs(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, seedUsage)
		return 2
	}
	if cfg.Env == config.EnvProduction {
		_, _ = fmt.Fprintln(stderr, "monacoctl seed: refused: production")
		return 2
	}
	raw, err := testkit.Scenario(name)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl seed: %v\n", err)
		return 2
	}
	if want := testkit.WantUsers(raw); len(users) != want {
		_, _ = fmt.Fprintf(stderr, "monacoctl seed: want %d users\n", want)
		return 2
	}
	ctx := context.Background()
	pool, err := open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	if err := requireUsers(ctx, pool, users); err != nil {
		return fail(stderr, err)
	}
	applied, err := applySeed(ctx, cfg, pool, name, raw, users)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "seeded %s: %d events applied\n", name, applied)
	return 0
}

func parseSeedArgs(args []string) (string, []ids.UserID, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, false
	}
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	list := fs.String("users", "", "")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *list == "" {
		return "", nil, false
	}
	var users []ids.UserID
	for raw := range strings.SplitSeq(*list, ",") {
		user, err := ids.ParseUserID(raw)
		if err != nil {
			return "", nil, false
		}
		users = append(users, user)
	}
	return args[0], users, true
}

type missingUserError struct{ id ids.UserID }

func (e missingUserError) Error() string { return "user " + e.id.String() + " not found" }

func requireUsers(ctx context.Context, pool *pgxpool.Pool, users []ids.UserID) error {
	for _, user := range users {
		var live bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL)`,
			user.UUID()).Scan(&live)
		if err != nil {
			return fmt.Errorf("monacoctl seed: read users: %w", err)
		}
		if !live {
			return missingUserError{user}
		}
	}
	return nil
}

type valuationRunner interface {
	RunOnce(ctx context.Context) error
}

func applySeed(
	ctx context.Context, cfg config.Config, pool *pgxpool.Pool, name string, raw []byte, users []ids.UserID,
) (int, error) {
	uow := db.New(pool, ids.Real{}, clock.Real{})
	set := registeredSet(cfg, pool, uow, clock.Real{})
	var consumers []bus.Consumer
	for _, c := range set.Consumers() {
		if slices.Contains(seedDurables(), c.Durable) {
			consumers = append(consumers, c)
		}
	}
	seeded, err := testkit.SeedEvents(ctx, pool, name, raw, users, consumers...)
	if err != nil || len(seeded) == 0 {
		return len(seeded), err
	}
	for _, m := range set {
		if runner, ok := m.(valuationRunner); ok {
			if err := runner.RunOnce(ctx); err != nil {
				return len(seeded), fmt.Errorf("monacoctl seed: value the boards: %w", err)
			}
		}
	}
	return len(seeded), nil
}
