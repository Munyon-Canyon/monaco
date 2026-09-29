package verify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

var errArgs = errors.New("bad arguments")

const Usage = "usage: monacoctl verify [flow <id> | all] [--outcome X] [--crash-at P]"

type Target struct {
	Flow    string
	Outcome string
	CrashAt string
}

func ParseArgs(args []string) (Target, error) {
	var t Target
	for i := 0; i < len(args); i++ {
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: %s needs a value", errArgs, args[i])
			}
			i++
			return args[i], nil
		}
		var err error
		switch args[i] {
		case "all":
			t.Flow = ""
		case "flow":
			t.Flow, err = next()
		case "--outcome":
			t.Outcome, err = next()
		case "--crash-at":
			t.CrashAt, err = next()
			if err == nil && !faultpoint.Known(t.CrashAt) {
				err = fmt.Errorf("%w: --crash-at %s is not a faultpoint", errArgs, t.CrashAt)
			}
		default:
			err = fmt.Errorf("%w: unknown argument %q", errArgs, args[i])
		}
		if err != nil {
			return Target{}, err
		}
	}
	return t, nil
}

type Config struct {
	Dir      string
	Environ  []string
	Go       string
	Docker   Docker
	Atlas    string
	TempDir  string
	Budget   Budget
	Postgres PostgresFunc
	Stdout   io.Writer
	Stderr   io.Writer
}

func Run(ctx context.Context, cfg Config, target Target) int {
	if err := run(ctx, cfg, target); err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "monacoctl verify: %v\n", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, cfg Config, target Target) (err error) {
	out, err := os.MkdirTemp(cfg.TempDir, "monaco-verify-bin-")
	if err != nil {
		return fmt.Errorf("binaries dir: %w", err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(out)) }()
	bins, err := build(ctx, cfg.Go, cfg.Dir, out, target.CrashAt != "")
	if err != nil {
		return err
	}
	if cfg.Postgres == nil {
		if err := cfg.Docker.ensureImage(ctx); err != nil {
			return err
		}
	}
	cover := filepath.Join(cfg.Dir, ".verify", "cover")
	if err := os.MkdirAll(cover, 0o750); err != nil {
		return fmt.Errorf("coverage dir: %w", err)
	}
	stack, err := Up(ctx, Options{
		Dir: cfg.Dir, Atlas: cfg.Atlas, Docker: cfg.Docker, Environ: cfg.Environ, Bins: bins,
		Budget: cfg.Budget, Postgres: cfg.Postgres, CoverDir: cover,
	})
	defer func() { err = errors.Join(err, stack.Down(ctx)) }()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cfg.Stdout, "stack %s healthy: api %s, worker %s\n", stack.RunID, stack.API, stack.Worker)
	return nil
}
