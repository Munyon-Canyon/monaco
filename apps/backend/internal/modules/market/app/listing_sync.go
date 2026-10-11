package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const listingVerifyLimit = 8

type ListingToVerify struct {
	ID             uuid.UUID
	Issuer         string
	Symbol         string
	Network        chain.Network
	TokenAddress   string
	WrapperAddress string
	Decimals       *uint8
}

type ListingVerifier interface {
	Verify(ctx context.Context, l ListingToVerify) (decimals uint8, err error)
}

type ListingCheck struct {
	ListingToVerify
	VerifiedDecimals uint8
	Err              error
}

func (c ListingCheck) Reason() string {
	var b strings.Builder
	b.WriteString(c.Err.Error())
	for _, a := range errs.Detail(c.Err) {
		b.WriteString(" " + a.String())
	}
	return b.String()
}

type ListingReport struct {
	Verified []ListingCheck
	Failed   []ListingCheck
}

type ListingState struct {
	ID           uuid.UUID
	Issuer       string
	Symbol       string
	Network      chain.Network
	TokenAddress string
	Enabled      bool
}

type ListingSync struct {
	uow      *db.UnitOfWork
	ids      ids.Generator
	clock    clock.Clock
	sources  []ListingSource
	verifier ListingVerifier
}

func NewListingSync(
	uow *db.UnitOfWork, g ids.Generator, c clock.Clock, sources []ListingSource, verifier ListingVerifier,
) *ListingSync {
	return &ListingSync{uow: uow, ids: g, clock: c, sources: sources, verifier: verifier}
}

type fetchedBatch struct {
	batch ListingBatch
	err   error
}

func (s *ListingSync) Run(ctx context.Context) (int, error) {
	fetched, err := concurrency.FanOut(ctx, max(len(s.sources), 1), s.sources,
		func(ctx context.Context, src ListingSource) (fetchedBatch, error) {
			batch, err := src.Listings(ctx)
			return fetchedBatch{batch: batch, err: err}, nil
		})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.ListingSync.Run")
	}
	changed := 0
	var failed []error
	for _, f := range fetched {
		n := 0
		if f.err == nil {
			n, f.err = s.apply(ctx, f.batch)
		}
		changed += n
		failed = append(failed, f.err)
	}
	report, err := s.Verify(ctx, false)
	for _, c := range report.Failed {
		observability.Degraded(ctx, observability.MarketListingVerifyFailed, slog.String("issuer", c.Issuer),
			slog.String("symbol", c.Symbol), slog.String("network", string(c.Network)),
			slog.String("reason", c.Reason()))
	}
	return changed + len(report.Verified), errors.Join(append(failed, err)...)
}

func (s *ListingSync) apply(ctx context.Context, b ListingBatch) (int, error) {
	now := s.clock.Now()
	rows := sqlc.UpsertListingsParams{Issuer: b.Issuer, Now: now}
	for _, l := range b.Listings {
		rows.Ids = append(rows.Ids, s.ids.NewV7())
		rows.Tickers = append(rows.Tickers, l.Ticker)
		rows.Symbols = append(rows.Symbols, l.Symbol)
		rows.Networks = append(rows.Networks, string(l.Network))
		rows.ChainIds = append(rows.ChainIds, l.ChainID)
		rows.TokenAddresses = append(rows.TokenAddresses, l.TokenAddress)
		rows.WrapperAddresses = append(rows.WrapperAddresses, l.WrapperAddress)
		rows.Decimals = append(rows.Decimals, decimalsArg(l.Decimals))
		rows.SharesPerTokens = append(rows.SharesPerTokens, sharesArg(l, now))
		rows.IssuerTradables = append(rows.IssuerTradables, l.Tradable)
	}
	var changed int
	err := s.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		upserted, err := q.UpsertListings(ctx, rows)
		halted, haltErr := q.HaltMissingListings(ctx,
			sqlc.HaltMissingListingsParams{Issuer: b.Issuer, Symbols: b.Missing})
		changed = int(upserted) + len(halted)
		tx.AfterCommit(func(ctx context.Context) {
			for _, h := range halted {
				observability.Degraded(ctx, observability.MarketListingSourceMissing, slog.String("issuer", b.Issuer),
					slog.String("symbol", h.Symbol), slog.String("network", h.Network))
			}
		})
		return errors.Join(err, haltErr)
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.ListingSync.apply", slog.String("issuer", b.Issuer))
	}
	return changed, nil
}

func decimalsArg(d *uint8) int16 {
	if d == nil {
		return -1
	}
	return int16(*d)
}

func sharesArg(l IssuerListing, now time.Time) string {
	shares := l.SharesPerToken
	if l.PendingSharesPerToken != nil && !l.PendingAt.IsZero() && !now.Before(l.PendingAt) {
		shares = l.PendingSharesPerToken
	}
	if shares == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(shares.FloatString(36), "0"), ".")
}

func (s *ListingSync) Verify(ctx context.Context, all bool) (ListingReport, error) {
	const op = "market.ListingSync.Verify"
	var rows []sqlc.ListListingsToVerifyRow
	err := s.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		rows, err = sqlc.New(tx.Queries()).ListListingsToVerify(ctx, all)
		return err
	})
	if err != nil {
		return ListingReport{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	checks, err := concurrency.FanOut(ctx, listingVerifyLimit, rows,
		func(ctx context.Context, row sqlc.ListListingsToVerifyRow) (ListingCheck, error) {
			return s.check(ctx, row, all)
		})
	if err != nil {
		return ListingReport{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	var report ListingReport
	for _, c := range checks {
		if c.Err != nil {
			report.Failed = append(report.Failed, c)
			continue
		}
		report.Verified = append(report.Verified, c)
	}
	return report, nil
}

func (s *ListingSync) check(ctx context.Context, row sqlc.ListListingsToVerifyRow, all bool) (ListingCheck, error) {
	l := ListingToVerify{
		ID: row.ID, Issuer: row.Issuer, Symbol: row.Symbol, Network: chain.Network(row.Network),
		TokenAddress: row.TokenAddress, WrapperAddress: row.WrapperAddress.String,
	}
	if d := row.Decimals.Int16; row.Decimals.Valid && d >= 0 && d <= math.MaxUint8 {
		stored := uint8(d)
		l.Decimals = &stored
	}
	decimals, verifyErr := s.verifier.Verify(ctx, l)
	if verifyErr == nil && l.Decimals != nil && *l.Decimals != decimals {
		verifyErr = errs.New(errs.CodeDecodeFailed, "market.ListingSync.check",
			slog.Int("issuer_decimals", int(*l.Decimals)), slog.Int("chain_decimals", int(decimals)))
	}
	c := ListingCheck{ListingToVerify: l, VerifiedDecimals: decimals, Err: verifyErr}
	if verifyErr != nil && !all {
		return c, nil
	}
	return c, s.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if verifyErr != nil {
			_, err := q.MarkListingUnverified(ctx, row.ID)
			return err
		}
		_, err := q.MarkListingVerified(ctx,
			sqlc.MarkListingVerifiedParams{Decimals: int16(decimals), VerifiedAt: s.clock.Now(), ID: row.ID})
		return err
	})
}

func SetListingOverride(ctx context.Context, q sqlc.DBTX, id uuid.UUID, o domain.Override) (ListingState, error) {
	const op = "market.SetListingOverride"
	row, err := sqlc.New(q).SetListingTradableOverride(ctx, sqlc.SetListingTradableOverrideParams{
		Override: string(o), ID: id,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ListingState{}, errs.New(errs.CodeNotFound, op, slog.String("listing_id", id.String()))
	case err != nil:
		return ListingState{}, errs.Wrap(err, errs.CodeOf(err), op, slog.String("listing_id", id.String()))
	}
	return ListingState{
		ID: row.ID, Issuer: row.Issuer, Symbol: row.Symbol, Network: chain.Network(row.Network),
		TokenAddress: row.TokenAddress, Enabled: row.Enabled,
	}, nil
}
