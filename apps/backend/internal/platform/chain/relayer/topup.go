package relayer

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	TopUpFloorLamports  = 2_000_000
	TopUpTargetLamports = 4_000_000
	TopUpPoll           = 2 * time.Second

	topUpOp        = "relayer.TopUps.TopUpSOL"
	systemTransfer = 2
	topUpLock      = `SELECT pg_advisory_xact_lock(hashtextextended('sol-topup:' || $1::text, 0))`
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
}

func NewTopUps(r *Relayer, rpc TopUpRPC, uow *db.UnitOfWork, clk clock.Clock) *TopUps {
	return &TopUps{relayer: r, rpc: rpc, uow: uow, clock: clk}
}

func (t *TopUps) TopUpSOL(ctx context.Context, wallet chain.SolanaAddress) error {
	if _, err := chain.ParseAddress(string(wallet)); err != nil {
		return errs.Wrap(err, errs.CodeInvalidAddress, topUpOp)
	}
	if low, err := t.below(ctx, wallet); err != nil || !low {
		return err
	}
	return t.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, topUpLock, string(wallet)); err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, topUpOp)
		}
		balance, err := t.rpc.SOLBalance(ctx, wallet)
		if err != nil || balance.Uint64() >= TopUpFloorLamports {
			return err
		}
		return t.send(ctx, wallet, TopUpTargetLamports-balance.Uint64())
	})
}

func (t *TopUps) below(ctx context.Context, wallet chain.SolanaAddress) (bool, error) {
	balance, err := t.rpc.SOLBalance(ctx, wallet)
	return err == nil && balance.Uint64() < TopUpFloorLamports, err
}

func (t *TopUps) send(ctx context.Context, wallet chain.SolanaAddress, lamports uint64) error {
	payer := t.relayer.address
	own, err := t.rpc.SOLBalance(ctx, payer)
	if err != nil {
		return err
	}
	if own.Uint64() <= FloorLamports+lamports {
		observability.Alert(ctx, observability.RelayerTopUpUnderfunded,
			slog.String("relayer", string(payer)), slog.String("wallet", string(wallet)),
			slog.Uint64("relayer_lamports", own.Uint64()), slog.Uint64("lamports", lamports),
			slog.Uint64("floor_lamports", FloorLamports))
		return errs.New(errs.CodeUpstreamUnavailable, topUpOp, slog.String("reason", "relayer_underfunded"),
			slog.Uint64("relayer_lamports", own.Uint64()))
	}
	hash, err := t.rpc.LatestBlockhash(ctx)
	if err != nil {
		return err
	}
	msg := transferSOL(payer, wallet, lamports, hash.Hash)
	tx := chain.Transaction{Signatures: [][]byte{ed25519.Sign(t.relayer.key, msg)}, Message: msg}
	sig, err := t.rpc.SendTransaction(ctx, tx.Encode())
	if err != nil {
		return err
	}
	if err := t.confirm(ctx, wallet, sig, hash.LastValidBlockHeight); err != nil {
		return err
	}
	observability.Info(ctx, observability.RelayerSOLToppedUp, slog.String("wallet", string(wallet)),
		slog.Uint64("lamports", lamports), slog.String("signature", string(sig)))
	return nil
}

func (t *TopUps) confirm(ctx context.Context, wallet chain.SolanaAddress, sig chain.Signature, lastValid uint64) error {
	for {
		statuses, err := t.rpc.SignatureStatuses(ctx, []chain.Signature{sig})
		if err != nil {
			return err
		}
		status := statuses[0]
		switch {
		case status.Failed, status.BlockhashExpired(lastValid):
			return errs.New(errs.CodeUpstreamUnavailable, topUpOp, slog.String("signature", string(sig)),
				slog.Bool("failed", status.Failed))
		case status.State != solana.StateNotFound:
			if low, err := t.below(ctx, wallet); err != nil || !low {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamTimeout, topUpOp)
		case <-t.clock.After(TopUpPoll):
		}
	}
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
