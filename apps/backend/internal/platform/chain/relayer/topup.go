package relayer

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/binary"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	TopUpFloorLamports  = 2_000_000
	TopUpTargetLamports = 4_000_000
	TopUpPoll           = 2 * time.Second
	TopUpCap            = 3
	TopUpCapWindow      = 24 * time.Hour

	topUpOp        = "relayer.TopUps.TopUpSOL"
	systemTransfer = 2

	topUpConfirmed = "confirmed"
	topUpExpired   = "expired"
)

type TopUpRPC interface {
	SOLBalance(ctx context.Context, addr chain.SolanaAddress) (money.BaseUnits, error)
	LatestBlockhash(ctx context.Context) (solana.Blockhash, error)
	SendTransaction(ctx context.Context, tx []byte) (chain.Signature, error)
	SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]solana.Status, error)
}

type TopUps struct {
	relayer *Relayer
	rpc     TopUpRPC
	uow     *db.UnitOfWork
	clock   clock.Clock
	ids     ids.Generator
}

func NewTopUps(r *Relayer, rpc TopUpRPC, uow *db.UnitOfWork, clk clock.Clock, g ids.Generator) *TopUps {
	return &TopUps{relayer: r, rpc: rpc, uow: uow, clock: clk, ids: g}
}

type topUp struct {
	id        uuid.UUID
	wallet    chain.SolanaAddress
	lamports  uint64
	raw       []byte
	signature chain.Signature
	lastValid uint64
}

type journalView struct {
	pending   uuid.UUID
	signature chain.Signature
	lastValid uint64
	recent    int32
}

type topUpPlan struct {
	resolve string
	next    *topUp
}

func (t *TopUps) TopUpSOL(ctx context.Context, wallet chain.SolanaAddress) error {
	if _, err := chain.ParseAddress(string(wallet)); err != nil {
		return errs.Wrap(err, errs.CodeInvalidAddress, topUpOp)
	}
	if low, err := t.below(ctx, wallet); err != nil || !low {
		return err
	}
	since := t.clock.Now().Add(-TopUpCapWindow)
	var seen journalView
	if err := t.uow.Do(ctx, func(ctx context.Context, tx db.Tx) (err error) {
		seen, err = t.view(ctx, sqlc.New(tx.Queries()), wallet, since)
		return err
	}); err != nil {
		return err
	}
	plan, err := t.plan(ctx, wallet, seen)
	if err != nil || plan == (topUpPlan{}) {
		return err
	}
	err = t.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if err := q.LockTopUp(ctx, string(wallet)); err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
		}
		now, err := t.view(ctx, q, wallet, since)
		if err != nil {
			return err
		}
		if now != seen {
			return errs.New(errs.CodeUpstreamUnavailable, topUpOp, slog.String("reason", "topup_changed"),
				slog.String("wallet", string(wallet)))
		}
		return t.apply(ctx, q, wallet, seen, plan)
	})
	if err != nil || plan.next == nil {
		return err
	}
	return t.broadcast(ctx, *plan.next)
}

func (t *TopUps) below(ctx context.Context, wallet chain.SolanaAddress) (bool, error) {
	balance, err := t.rpc.SOLBalance(ctx, wallet)
	return err == nil && balance.Uint64() < TopUpFloorLamports, err
}

func (t *TopUps) view(
	ctx context.Context,
	q *sqlc.Queries,
	wallet chain.SolanaAddress,
	since time.Time,
) (journalView, error) {
	var seen journalView
	row, err := q.LatestPendingTopUp(ctx, string(wallet))
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return journalView{}, errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
	default:
		lastValid, err := strconv.ParseUint(row.LastValidBlockHeight, 10, 64)
		if err != nil {
			return journalView{}, errs.Wrap(
				err,
				errs.CodeDecodeFailed,
				topUpOp,
				slog.String("signature", row.Signature),
			)
		}
		seen = journalView{pending: row.ID, signature: chain.Signature(row.Signature), lastValid: lastValid}
	}
	seen.recent, err = q.CountTopUpsSince(ctx, sqlc.CountTopUpsSinceParams{Wallet: string(wallet), Since: since})
	if err != nil {
		return journalView{}, errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
	}
	return seen, nil
}

func (t *TopUps) plan(ctx context.Context, wallet chain.SolanaAddress, seen journalView) (topUpPlan, error) {
	var plan topUpPlan
	if seen.pending != uuid.Nil {
		status, err := t.status(ctx, seen.signature)
		if err != nil {
			return topUpPlan{}, err
		}
		switch {
		case status.Failed || status.BlockhashExpired(seen.lastValid):
			plan.resolve = topUpExpired
		case status.State != solana.StateNotFound:
			return topUpPlan{resolve: topUpConfirmed}, nil
		default:
			return topUpPlan{}, errs.New(
				errs.CodeUpstreamUnavailable,
				topUpOp,
				slog.String("reason", "topup_in_flight"),
				slog.String("signature", string(seen.signature)),
			)
		}
	}
	balance, err := t.rpc.SOLBalance(ctx, wallet)
	if err != nil || balance.Uint64() >= TopUpFloorLamports {
		return plan, err
	}
	if err := t.underCap(ctx, wallet, seen.recent); err != nil {
		return topUpPlan{}, err
	}
	next, err := t.build(ctx, wallet, TopUpTargetLamports-balance.Uint64())
	if err != nil {
		return topUpPlan{}, err
	}
	plan.next = &next
	return plan, nil
}

func (t *TopUps) apply(
	ctx context.Context,
	q *sqlc.Queries,
	wallet chain.SolanaAddress,
	seen journalView,
	plan topUpPlan,
) error {
	if plan.resolve != "" {
		if err := resolve(ctx, q, seen.pending, plan.resolve, t.clock.Now()); err != nil {
			return err
		}
	}
	next := plan.next
	if next == nil {
		return nil
	}
	err := q.InsertTopUp(ctx, sqlc.InsertTopUpParams{
		ID: next.id, Wallet: string(wallet), Lamports: strconv.FormatUint(next.lamports, 10),
		Signature: string(next.signature), LastValidBlockHeight: strconv.FormatUint(next.lastValid, 10),
		CreatedAt: t.clock.Now(),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
	}
	return nil
}

func (t *TopUps) underCap(ctx context.Context, wallet chain.SolanaAddress, count int32) error {
	if count < TopUpCap {
		return nil
	}
	observability.Alert(ctx, observability.RelayerTopUpCapped,
		slog.String("wallet", string(wallet)), slog.Int("topups", int(count)))
	return errs.New(errs.CodeUpstreamUnavailable, topUpOp, slog.String("reason", "topup_capped"),
		slog.String("wallet", string(wallet)))
}

func (t *TopUps) build(ctx context.Context, wallet chain.SolanaAddress, lamports uint64) (topUp, error) {
	payer := t.relayer.address
	own, err := t.rpc.SOLBalance(ctx, payer)
	if err != nil {
		return topUp{}, err
	}
	if own.Uint64() <= FloorLamports+lamports {
		observability.Alert(ctx, observability.RelayerTopUpUnderfunded,
			slog.String("relayer", string(payer)), slog.String("wallet", string(wallet)),
			slog.Uint64("relayer_lamports", own.Uint64()), slog.Uint64("lamports", lamports),
			slog.Uint64("floor_lamports", FloorLamports))
		return topUp{}, errs.New(errs.CodeUpstreamUnavailable, topUpOp,
			slog.String("reason", "relayer_underfunded"), slog.Uint64("relayer_lamports", own.Uint64()))
	}
	hash, err := t.rpc.LatestBlockhash(ctx)
	if err != nil {
		return topUp{}, err
	}
	msg := transferSOL(payer, wallet, lamports, hash.Hash)
	signature := ed25519.Sign(t.relayer.key, msg)
	return topUp{
		id: t.ids.NewV7(), wallet: wallet, lamports: lamports, signature: chain.SignatureOf(signature),
		raw:       chain.Transaction{Signatures: [][]byte{signature}, Message: msg}.Encode(),
		lastValid: hash.LastValidBlockHeight,
	}, nil
}

func (t *TopUps) broadcast(ctx context.Context, next topUp) error {
	if _, err := t.rpc.SendTransaction(ctx, next.raw); err != nil {
		return err
	}
	for {
		status, err := t.status(ctx, next.signature)
		if err != nil {
			return err
		}
		switch {
		case status.Failed, status.BlockhashExpired(next.lastValid):
			return errors.Join(t.record(ctx, next, topUpExpired), errs.New(errs.CodeUpstreamUnavailable, topUpOp,
				slog.String("signature", string(next.signature)), slog.Bool("failed", status.Failed)))
		case status.State != solana.StateNotFound:
			low, err := t.below(ctx, next.wallet)
			if err != nil {
				return err
			}
			if !low {
				return t.landed(ctx, next)
			}
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamTimeout, topUpOp)
		case <-t.clock.After(TopUpPoll):
		}
	}
}

func (t *TopUps) landed(ctx context.Context, next topUp) error {
	if err := t.record(ctx, next, topUpConfirmed); err != nil {
		return err
	}
	observability.Info(ctx, observability.RelayerSOLToppedUp, slog.String("wallet", string(next.wallet)),
		slog.Uint64("lamports", next.lamports), slog.String("signature", string(next.signature)))
	return nil
}

func (t *TopUps) record(ctx context.Context, next topUp, status string) error {
	return t.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return resolve(ctx, sqlc.New(tx.Queries()), next.id, status, t.clock.Now())
	})
}

func resolve(ctx context.Context, q *sqlc.Queries, id uuid.UUID, status string, at time.Time) error {
	if _, err := q.ResolveTopUp(ctx, sqlc.ResolveTopUpParams{ID: id, Status: status, At: at}); err != nil {
		return errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
	}
	return nil
}

func (t *TopUps) status(ctx context.Context, sig chain.Signature) (solana.Status, error) {
	statuses, err := t.rpc.SignatureStatuses(ctx, []chain.Signature{sig})
	if err != nil {
		return solana.Status{}, err
	}
	return statuses[0], nil
}

func transferSOL(payer, to chain.SolanaAddress, lamports uint64, blockhash [32]byte) []byte {
	keys := []chain.SolanaAddress{payer, to, chain.SystemProgram}
	msg := slices.Concat([]byte{1, 0, 1}, chain.CompactU16(len(keys)))
	for _, k := range keys {
		b, _ := k.Bytes()
		msg = append(msg, b...)
	}
	msg = append(msg, blockhash[:]...)
	data := binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint32(nil, systemTransfer), lamports)
	return append(append(msg, 1), instruction(2, []byte{0, 1}, data)...)
}
