package authn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

var _ auth.TokenVerifier = (*Verifier)(nil)

type route func(ctx context.Context, raw string) (auth.Actor, error)

type Verifier struct {
	privy   app.PrivyUsers
	queries *sqlc.Queries
	routes  map[string]route
}

func New(cfg config.Config, clk clock.Clock, privy app.PrivyUsers, db sqlc.DBTX) (*Verifier, error) {
	v := &Verifier{privy: privy, queries: sqlc.New(db)}
	v.routes = map[string]route{"ES256": v.privyActor}
	if cfg.Env != config.EnvProduction {
		dev, err := auth.NewDevVerifier(cfg, clk)
		if err != nil {
			return nil, err
		}
		v.routes["HS256"] = dev.Verify
	}
	return v, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (auth.Actor, error) {
	const op = "identity.Verifier.Verify"
	alg, ok := algorithm(raw)
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "malformed"))
	}
	verify, ok := v.routes[alg]
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "algorithm"))
	}
	return verify(ctx, raw)
}

func (v *Verifier) VerifyPrivy(ctx context.Context, raw string) (app.PrivyUserID, error) {
	return v.privy.Verify(ctx, raw)
}

func (v *Verifier) privyActor(ctx context.Context, raw string) (auth.Actor, error) {
	const op = "identity.Verifier.privyActor"
	id, err := v.privy.Verify(ctx, raw)
	if err != nil {
		return auth.Actor{}, err
	}
	row, err := v.queries.ActorByPrivyUserID(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Actor{}, errs.New(errs.CodeSessionRequired, op)
	}
	if err != nil {
		return auth.Actor{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return auth.Actor{Kind: auth.ActorUser, ID: row.ID.String(), Standing: auth.Standing(row.AccountStatus)}, nil
}

func algorithm(raw string) (string, bool) {
	segment, _, _ := strings.Cut(raw, ".")
	header, err := base64.RawURLEncoding.DecodeString(segment)
	var h struct {
		Alg string `json:"alg"`
	}
	if err != nil || json.Unmarshal(header, &h) != nil {
		return "", false
	}
	return h.Alg, true
}
