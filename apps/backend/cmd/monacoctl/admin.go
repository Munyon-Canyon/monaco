package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const adminUsage = "usage: monacoctl admin grant --handle <handle> --role viewer|moderator|operator | revoke --handle <handle>"

func toolAdmin(env toolEnv) tool { return adminTool(env.environ, clock.Real{}) }

func adminTool(environ []string, clk clock.Clock) tool {
	grant := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("admin grant", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		handle, role := fs.String("handle", "", ""), fs.String("role", "", "")
		if fs.Parse(args) != nil || fs.NArg() != 0 || *handle == "" {
			_, _ = fmt.Fprintln(stderr, adminUsage)
			return 2
		}
		parsed, ok := adminRole(*role)
		if !ok {
			_, _ = fmt.Fprintln(stderr, adminUsage)
			return 2
		}
		id, err := changeAdmin(cfg, clk, *handle, parsed, false)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, id)
		return 0
	}
	revoke := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("admin revoke", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		handle := fs.String("handle", "", "")
		if fs.Parse(args) != nil || fs.NArg() != 0 || *handle == "" {
			_, _ = fmt.Fprintln(stderr, adminUsage)
			return 2
		}
		id, err := changeAdmin(cfg, clk, *handle, "", true)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, id)
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"grant": grant, "revoke": revoke}, nil, environ, args, stdout, stderr)
	}
}

func adminRole(raw string) (domain.Role, bool) {
	role := domain.Role(raw)
	return role, role == domain.RoleViewer || role == domain.RoleModerator || role == domain.RoleOperator
}

func changeAdmin(cfg config.Config, clk clock.Clock, handle string, role domain.Role, revoke bool) (ids.UserID, error) {
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return ids.UserID{}, err
	}
	defer pool.Close()
	var id ids.UserID
	err = db.New(pool, ids.Real{}, clk).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		parsed, err := adminUserID(ctx, tx, handle)
		if err != nil {
			return err
		}
		id = parsed
		if revoke {
			return revokeAdmin(ctx, tx, id, clk.Now())
		}
		return grantAdmin(ctx, tx, id, role, clk.Now())
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UserID{}, fmt.Errorf("admin: %w", err)
	}
	return id, err
}

func adminUserID(ctx context.Context, tx db.Tx, handle string) (ids.UserID, error) {
	const byHandle = `SELECT id FROM users WHERE handle = $1 AND deleted_at IS NULL`
	var rawID string
	err := tx.Queries().QueryRow(ctx, byHandle, handle).Scan(&rawID)
	if err != nil {
		return ids.UserID{}, fmt.Errorf("admin user by handle: %w", err)
	}
	return ids.ParseUserID(rawID)
}

func grantAdmin(ctx context.Context, tx db.Tx, id ids.UserID, role domain.Role, at time.Time) error {
	const grant = `INSERT INTO admins (user_id, role, granted_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role, granted_at = EXCLUDED.granted_at, revoked_at = NULL`
	_, err := tx.Queries().Exec(ctx, grant, id.UUID(), string(role), at)
	if err != nil {
		return err
	}
	return tx.Events.Append(ctx, events.AdminGranted{V: 1, UserID: id.UUID(), Role: string(role)})
}

func revokeAdmin(ctx context.Context, tx db.Tx, id ids.UserID, at time.Time) error {
	const revoke = `UPDATE admins SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL`
	result, err := tx.Queries().Exec(ctx, revoke, at, id.UUID())
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Events.Append(ctx, events.AdminRevoked{V: 1, UserID: id.UUID()})
}
