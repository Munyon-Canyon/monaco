package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const opsUsage = "usage: monacoctl ops pause --cabal <id>|--all --note <text> | resume --cabal <id>|--all"

func toolOps(env toolEnv) tool { return opsTool(env.environ, clock.Real{}) }

type pauseScope struct {
	cabal *ids.CabalID
	label string
}

func parsePauseScope(fs *flag.FlagSet, args []string) (pauseScope, bool) {
	rawCabal := fs.String("cabal", "", "")
	all := fs.Bool("all", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || (*rawCabal == "") == !*all {
		return pauseScope{}, false
	}
	if *all {
		return pauseScope{label: "all"}, true
	}
	id, err := ids.ParseCabalID(*rawCabal)
	if err != nil {
		return pauseScope{}, false
	}
	return pauseScope{cabal: &id, label: id.String()}, true
}

func opsTool(environ []string, clk clock.Clock) tool {
	withFunding := func(cfg config.Config, stderr io.Writer, fn func(context.Context, *funding.Module) error) int {
		ctx := context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clk)}
		if err := fn(ctx, funding.New(deps)); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	pause := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("ops pause", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		note := fs.String("note", "", "")
		scope, ok := parsePauseScope(fs, args)
		if !ok || strings.TrimSpace(*note) == "" {
			_, _ = fmt.Fprintln(stderr, opsUsage)
			return 2
		}
		return withFunding(cfg, stderr, func(ctx context.Context, m *funding.Module) error {
			id, err := m.PauseFromOps(ctx, scope.cabal, *note)
			if err == nil {
				_, _ = fmt.Fprintf(stdout, "paused %s\tpause_id=%s\n", scope.label, id)
			}
			return err
		})
	}
	resume := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("ops resume", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		scope, ok := parsePauseScope(fs, args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, opsUsage)
			return 2
		}
		return withFunding(cfg, stderr, func(ctx context.Context, m *funding.Module) error {
			err := m.ResumeFromOps(ctx, scope.cabal)
			if err == nil {
				_, _ = fmt.Fprintf(stdout, "resumed %s\n", scope.label)
			}
			return err
		})
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"pause": pause, "resume": resume}, nil, environ, args, stdout, stderr)
	}
}
