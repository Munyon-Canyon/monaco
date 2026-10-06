package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const notifyUsage = "usage: monacoctl notify test --user <id>"

func toolNotify(env toolEnv) tool { return notifyTool(env.environ, clock.Real{}) }

type lastID struct {
	ids.Generator
	last uuid.UUID
}

func (g *lastID) NewV7() uuid.UUID {
	g.last = g.Generator.NewV7()
	return g.last
}

func notifyTool(environ []string, clk clock.Clock) tool {
	test := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("notify test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		rawUser := fs.String("user", "", "")
		parsed := fs.Parse(args)
		user, err := ids.ParseUserID(*rawUser)
		if parsed != nil || err != nil || fs.NArg() != 0 {
			_, _ = fmt.Fprintln(stderr, notifyUsage)
			return 2
		}
		ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		gen := &lastID{Generator: ids.Real{}}
		if err := db.New(pool, gen, clk).Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return tx.Events.Append(ctx, events.NotifyTestRequested{V: 1, UserID: user.UUID()})
		}); err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, gen.last)
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"test": test}, nil, environ, args, stdout, stderr)
	}
}
