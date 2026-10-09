package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

const pruneMinAge = 24 * time.Hour

func devUser(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "delete" {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	id, err := ids.ParseUserID(args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, devUserSubjectLine)
		return 2
	}
	if err := testflows.DeleteDevUser(context.Background(), cfg, id); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev user delete: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "deleted dev user %s\n", id)
	return 0
}

func devUsers(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "prune" {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	fs := flag.NewFlagSet("dev users prune", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dry := fs.Bool("dry-run", false, "list the Privy users prune would delete")
	apply := fs.Bool("apply", false, "delete them")
	if err := fs.Parse(args[1:]); err != nil || *dry == *apply || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if cfg.Env.Deployed() {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev users prune: refused with MONACO_ENV=%s\n", cfg.Env)
		return 1
	}
	if err := pruneDevUsers(context.Background(), cfg, *apply, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev users prune: %v\n", err)
		return 1
	}
	return 0
}

func pruneDevUsers(ctx context.Context, cfg config.Config, apply bool, stdout io.Writer) error {
	clk := clock.Real{}
	client, err := chainprivy.New(cfg, clk)
	if err != nil {
		return err
	}
	users, err := client.ListUsers(ctx)
	if err != nil {
		return err
	}
	cutoff := clk.Now().Add(-pruneMinAge)
	n := 0
	for _, u := range users {
		if _, dev := domain.DevSuffix(u.Email); !dev || u.Accounts != 1 || !u.CreatedAt.Before(cutoff) {
			continue
		}
		n++
		_, _ = fmt.Fprintf(stdout, "%s %s created %s\n", u.ID, u.Email, u.CreatedAt.Format(time.RFC3339))
		if apply {
			if err := client.DeleteUser(ctx, u.ID); err != nil {
				return err
			}
		}
	}
	verb := "would delete"
	if apply {
		verb = "deleted"
	}
	_, _ = fmt.Fprintf(stdout, "%s %d of %d Privy users\n", verb, n, len(users))
	return nil
}
