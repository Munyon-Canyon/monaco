package app

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	rankingdomain "github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	liveSampleAge     = 2 * time.Hour
	backfillSampleAge = 36 * time.Hour
	preIPOSampleAge   = 36 * time.Hour
	liveSampleWindow  = 7 * 24 * time.Hour
	unpricedAfter     = 6 * time.Hour
	CallsBatch        = 200

	preIPOKind      = "pre_ipo"
	refSourceFill   = "fill"
	refSourceMarket = "market"
)

type ResolveCalls struct {
	db     dbsqlc.DBTX
	uow    *db.UnitOfWork
	market Market
}

func NewResolveCalls(pool dbsqlc.DBTX, uow *db.UnitOfWork, m Market) ResolveCalls {
	return ResolveCalls{db: pool, uow: uow, market: m}
}

type ResolveReport struct{ Scanned, Resolved, Voided int }

func (r ResolveCalls) Run(ctx context.Context, now time.Time, limit int32) (ResolveReport, error) {
	due, err := sqlc.New(r.db).DueCalls(ctx, sqlc.DueCallsParams{Now: now, N: limit})
	if err != nil {
		return ResolveReport{}, err
	}
	if len(due) == 0 {
		return ResolveReport{}, nil
	}
	judged, err := r.judge(ctx, due, now)
	if err != nil {
		return ResolveReport{}, err
	}
	report := ResolveReport{Scanned: len(due)}
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		report.Resolved, report.Voided = 0, 0
		q := sqlc.New(tx.Queries())
		for _, j := range judged {
			done, err := j.apply(ctx, q, tx.Events, now)
			if err != nil {
				return err
			}
			switch {
			case !done:
			case j.void:
				report.Voided++
			default:
				report.Resolved++
			}
		}
		return nil
	})
	return report, err
}

type priced struct {
	micros money.Micros
	raw    int64
}

type judgement struct {
	call     sqlc.Call
	void     bool
	ref      priced
	source   string
	end      priced
	moveBps  int32
	yesRight bool
}

func (r ResolveCalls) judge(ctx context.Context, due []sqlc.Call, now time.Time) ([]judgement, error) {
	assets, err := r.assetsOf(ctx, due)
	if err != nil {
		return nil, err
	}
	out := make([]judgement, 0, len(due))
	for _, call := range due {
		ref, source, ok := reference(call)
		if !ok {
			out = append(out, judgement{call: call, void: true})
			continue
		}
		end, found, err := r.endPrice(ctx, call, assets, now)
		if err != nil {
			return nil, err
		}
		if !found {
			if now.After(call.ResolveAt.Time.Add(unpricedAfter)) {
				out = append(out, judgement{call: call, void: true})
			}
			continue
		}
		bps, err := rankingdomain.MoveBps(ref.micros, end.micros)
		if err != nil || bps > math.MaxInt32 || bps < math.MinInt32 {
			out = append(out, judgement{call: call, void: true})
			continue
		}
		out = append(out, judgement{
			call: call, ref: ref, source: source, end: end, moveBps: int32(bps),
			yesRight: rankingdomain.Right(call.Kind.String, rankingdomain.SideYes, ref.micros, end.micros),
		})
	}
	return out, nil
}

func (r ResolveCalls) assetsOf(ctx context.Context, due []sqlc.Call) (map[uuid.UUID]market.Asset, error) {
	var mints []string
	for _, c := range due {
		if c.Mint.Valid && !slices.Contains(mints, c.Mint.String) {
			mints = append(mints, c.Mint.String)
		}
	}
	out := map[uuid.UUID]market.Asset{}
	if len(mints) == 0 {
		return out, nil
	}
	assets, err := r.market.AssetsByMints(ctx, mints)
	if err != nil {
		return nil, err
	}
	for _, a := range assets {
		out[a.ID.UUID()] = a
	}
	return out, nil
}

func (r ResolveCalls) endPrice(
	ctx context.Context, call sqlc.Call, assets map[uuid.UUID]market.Asset, now time.Time,
) (priced, bool, error) {
	if !call.AssetID.Valid {
		return priced{}, false, nil
	}
	id := market.AssetIDFrom(call.AssetID.Bytes)
	prices, err := r.market.PricesAsOf(ctx, []market.AssetID{id}, call.ResolveAt.Time)
	if err != nil {
		return priced{}, false, err
	}
	sample, ok := prices[id]
	if !ok {
		return priced{}, false, nil
	}
	limit := liveSampleAge
	if now.Sub(call.ResolveAt.Time) > liveSampleWindow {
		limit = backfillSampleAge
	}
	if string(assets[call.AssetID.Bytes].Kind) == preIPOKind {
		limit = preIPOSampleAge
	}
	raw := sample.Micros.Uint64()
	if raw > math.MaxInt64 || call.ResolveAt.Time.Sub(sample.ObservedAt) > limit {
		return priced{}, false, nil
	}
	return priced{micros: sample.Micros, raw: int64(raw)}, true, nil
}

func reference(call sqlc.Call) (priced, string, bool) {
	raw, source := call.FillPriceMicros, refSourceFill
	if !raw.Valid {
		raw, source = call.MarketRefMicros, refSourceMarket
	}
	if !call.Kind.Valid || !raw.Valid || raw.Int64 <= 0 {
		return priced{}, "", false
	}
	return priced{micros: money.MicrosFromUint64(uint64(raw.Int64)), raw: raw.Int64}, source, true
}

func (j judgement) apply(ctx context.Context, q *sqlc.Queries, log eventLog, now time.Time) (bool, error) {
	if j.void {
		n, err := q.VoidUnpriced(ctx, sqlc.VoidUnpricedParams{ProposalID: j.call.ProposalID, At: now})
		return n > 0, err
	}
	n, err := q.ResolveCall(ctx, sqlc.ResolveCallParams{
		RefPriceMicros: j.ref.raw, RefSource: j.source, EndPriceMicros: j.end.raw, MoveBps: j.moveBps,
		ResolvedAt: j.call.ResolveAt.Time, At: now, ProposalID: j.call.ProposalID,
	})
	if err != nil || n == 0 {
		return false, err
	}
	voters, err := q.SetVoterRight(ctx, sqlc.SetVoterRightParams{ProposalID: j.call.ProposalID, YesRight: j.yesRight})
	if err == nil {
		_, err = q.SetCabalCallRight(ctx, sqlc.SetCabalCallRightParams{
			ProposalID: j.call.ProposalID, YesRight: j.yesRight,
		})
	}
	if err == nil {
		err = log.Append(ctx, j.event(voters))
	}
	return true, err
}

type eventLog interface {
	Append(context.Context, events.Event) error
}

func (j judgement) event(voters []sqlc.SetVoterRightRow) events.CallResolved {
	right, missed := []uuid.UUID{}, []uuid.UUID{}
	for _, v := range voters {
		if v.IsRight {
			right = append(right, v.UserID)
		} else {
			missed = append(missed, v.UserID)
		}
	}
	return events.CallResolved{
		V:          1,
		ProposalID: j.call.ProposalID,
		CabalID:    j.call.CabalID,
		Kind:       j.call.Kind.String,
		Symbol:     j.call.Symbol.String,
		Outcome:    j.call.Outcome.String,
		ResolvedAt: j.call.ResolveAt.Time,
		MoveBps: int(
			j.moveBps,
		),
		RightVoterIDs:  right,
		MissedVoterIDs: missed,
		CabalRight:     j.yesRight == (j.call.Outcome.String == "passed"),
	}
}
