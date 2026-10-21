package relayer

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	NetworkFeeLamports       = 10_000
	TokenAccountRentLamports = 2_039_280
	SponsorCostLamports      = NetworkFeeLamports + TokenAccountRentLamports

	SponsorUserDailyLamports        = 5*SponsorCostLamports + 200*NetworkFeeLamports
	SponsorDestinationDailyLamports = 2*SponsorCostLamports + 200*NetworkFeeLamports
	SponsorGlobalDailyLamports      = 500*SponsorCostLamports + 50_000*NetworkFeeLamports
	SponsorWindow                   = 24 * time.Hour

	sponsorOp = "relayer.SponsorBudgets.Reserve"

	scopeUser        = "user"
	scopeDestination = "destination"
	scopeGlobal      = "global"
)

type SponsorBudgets struct {
	UserDaily        uint64
	DestinationDaily uint64
	GlobalDaily      uint64
}

type SponsorRequest struct {
	Ref         uuid.UUID
	User        ids.UserID
	Destination chain.SolanaAddress
	CanCreate   bool
	Now         time.Time
}

type SponsorPlan struct {
	Lamports      uint64
	CreateAccount bool
}

type budget struct {
	scope, key string
	limit      uint64
}

func (b SponsorBudgets) budgets(req SponsorRequest) []budget {
	return []budget{
		{scopeUser, req.User.String(), cmp.Or(b.UserDaily, SponsorUserDailyLamports)},
		{scopeDestination, string(req.Destination), cmp.Or(b.DestinationDaily, SponsorDestinationDailyLamports)},
		{scopeGlobal, "", cmp.Or(b.GlobalDaily, SponsorGlobalDailyLamports)},
	}
}

func (b SponsorBudgets) Reserve(ctx context.Context, conn sqlc.DBTX, req SponsorRequest) (SponsorPlan, error) {
	q := sqlc.New(conn)
	if plan, found, err := reserved(ctx, q, req.Ref); err != nil || found {
		return plan, err
	}
	budgets := b.budgets(req)
	for _, bg := range budgets {
		if err := q.LockSponsorScope(ctx, sqlc.LockSponsorScopeParams{Scope: bg.scope, ScopeKey: bg.key}); err != nil {
			return SponsorPlan{}, errs.Wrap(err, errs.CodeDBUnavailable, sponsorOp)
		}
	}
	plans := []SponsorPlan{{Lamports: NetworkFeeLamports}}
	if req.CanCreate {
		plans = []SponsorPlan{{Lamports: SponsorCostLamports, CreateAccount: true}, plans[0]}
	}
	for _, plan := range plans {
		fits, err := fits(ctx, q, budgets, plan.Lamports, req.Now)
		if err != nil {
			return SponsorPlan{}, err
		}
		if fits {
			return plan, insert(ctx, q, budgets, req, plan)
		}
	}
	return SponsorPlan{}, errs.New(errs.CodeRateLimited, sponsorOp, slog.String("reason", "sponsor_budget"),
		slog.String("user_id", req.User.String()), slog.String("destination", string(req.Destination)))
}

func (b SponsorBudgets) Release(ctx context.Context, conn sqlc.DBTX, ref uuid.UUID, at time.Time) error {
	_, err := sqlc.New(conn).ReleaseSponsorReservation(ctx, sqlc.ReleaseSponsorReservationParams{
		Ref: ref, ReleasedAt: at,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeDBUnavailable, "relayer.SponsorBudgets.Release")
	}
	return nil
}

func reserved(ctx context.Context, q *sqlc.Queries, ref uuid.UUID) (SponsorPlan, bool, error) {
	row, err := q.SponsorReservation(ctx, ref)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return SponsorPlan{}, false, nil
	case err != nil:
		return SponsorPlan{}, false, errs.Wrap(err, errs.CodeDBUnavailable, sponsorOp)
	}
	lamports, err := strconv.ParseUint(row.Lamports, 10, 64)
	if err != nil {
		return SponsorPlan{}, false, errs.Wrap(err, errs.CodeDecodeFailed, sponsorOp)
	}
	return SponsorPlan{Lamports: lamports, CreateAccount: row.Rent}, true, nil
}

func fits(ctx context.Context, q *sqlc.Queries, budgets []budget, cost uint64, now time.Time) (bool, error) {
	for _, bg := range budgets {
		text, err := q.SponsorSpent(ctx, sqlc.SponsorSpentParams{
			Scope: bg.scope, ScopeKey: bg.key, Since: now.Add(-SponsorWindow),
		})
		if err != nil {
			return false, errs.Wrap(err, errs.CodeDBUnavailable, sponsorOp)
		}
		spent, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return false, errs.Wrap(err, errs.CodeDecodeFailed, sponsorOp)
		}
		if spent > bg.limit || cost > bg.limit-spent {
			return false, nil
		}
	}
	return true, nil
}

func insert(ctx context.Context, q *sqlc.Queries, budgets []budget, req SponsorRequest, plan SponsorPlan) error {
	for _, bg := range budgets {
		err := q.InsertSponsorReservation(ctx, sqlc.InsertSponsorReservationParams{
			Ref: req.Ref, Scope: bg.scope, ScopeKey: bg.key, Lamports: strconv.FormatUint(plan.Lamports, 10),
			Rent: plan.CreateAccount, CreatedAt: req.Now,
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, sponsorOp)
		}
	}
	return nil
}
