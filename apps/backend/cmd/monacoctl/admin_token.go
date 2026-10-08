package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const adminNameUsage = "usage: monacoctl admin token create --name <name> | revoke --name <name>"

func adminTokenCmd(environ []string, clk clock.Clock, random io.Reader) command {
	createdBy := "monacoctl"
	for _, kv := range environ {
		if user, ok := strings.CutPrefix(kv, "USER="); ok && user != "" {
			createdBy = "monacoctl:" + user
		}
	}
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		verb, name, ok := parseAdminTokenArgs(args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, adminNameUsage)
			return 2
		}
		ctx := context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		q := sqlc.New(pool)
		if verb == "revoke" {
			if err := revokeServiceToken(ctx, q, clk, name); err != nil {
				return fail(stderr, err)
			}
			_, _ = fmt.Fprintln(stdout, name)
			return 0
		}
		issued, err := createServiceToken(ctx, q, clk, random, name, createdBy)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, issued)
		return 0
	}
}

func parseAdminTokenArgs(args []string) (verb, name string, ok bool) {
	if len(args) == 0 || (args[0] != "create" && args[0] != "revoke") {
		return "", "", false
	}
	fs := flag.NewFlagSet("admin token "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	raw := fs.String("name", "", "")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return "", "", false
	}
	name, err := domain.ParseServiceTokenName(*raw)
	return args[0], name, err == nil
}

func createServiceToken(
	ctx context.Context, q *sqlc.Queries, clk clock.Clock, random io.Reader, name, createdBy string,
) (string, error) {
	const op = "monacoctl.adminTokenCreate"
	issued, hash, err := domain.NewServiceToken(random)
	if err != nil {
		return "", err
	}
	err = q.InsertServiceToken(ctx, sqlc.InsertServiceTokenParams{
		ID: ids.Real{}.NewV7(), Name: name, TokenHash: hash, CreatedAt: clk.Now(), CreatedBy: createdBy,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return "", errs.Wrap(
			err,
			errs.CodeInvalidInput,
			op+": a live token already has the name "+name+", revoke it first",
		)
	}
	if err != nil {
		return "", errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return issued, nil
}

func revokeServiceToken(ctx context.Context, q *sqlc.Queries, clk clock.Clock, name string) error {
	const op = "monacoctl.adminTokenRevoke"
	n, err := q.RevokeServiceToken(ctx, sqlc.RevokeServiceTokenParams{Name: name, RevokedAt: clk.Now()})
	if err != nil {
		return errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	if n == 0 {
		return errs.Wrap(pgx.ErrNoRows, errs.CodeNotFound, op+": no live token named "+name)
	}
	return nil
}
