package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const createCabalOp = "cabal.CreateCabal"

type CreateCabal struct {
	ActorID        ids.UserID
	IdempotencyKey string
	Name           domain.Name
	Rules          domain.Rules
}

type CreatedCabal struct {
	ID              ids.CabalID
	Name            string
	TreasuryAddress chain.SolanaAddress
	PrivyWalletID   string
}

type CreateCabalDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Wallets TreasuryWallets
	IDs     ids.Generator
	Clock   clock.Clock
}

type CreateCabalHandler struct {
	uow     *db.UnitOfWork
	reads   sqlc.DBTX
	wallets TreasuryWallets
	ids     ids.Generator
	clock   clock.Clock
}

func NewCreateCabalHandler(d CreateCabalDeps) *CreateCabalHandler {
	return &CreateCabalHandler{
		uow: d.UoW, reads: d.Reads, wallets: d.Wallets, ids: d.IDs, clock: d.Clock,
	}
}

func TreasuryKey(actor ids.UserID, idempotencyKey string) string {
	return "cabal-treasury:" + actor.String() + ":" + idempotencyKey
}

func (h *CreateCabalHandler) Handle(ctx context.Context, cmd CreateCabal) (CreatedCabal, error) {
	if err := cmd.validate(); err != nil {
		return CreatedCabal{}, err
	}
	walletID, address, err := h.wallets.CreateAppOwned(ctx, TreasuryKey(cmd.ActorID, cmd.IdempotencyKey))
	if err != nil {
		return CreatedCabal{}, err
	}
	if created, found, err := h.committed(ctx, cmd, walletID); err != nil || found {
		return created, err
	}
	var created CreatedCabal
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var writeErr error
		created, writeErr = h.write(ctx, tx, cmd, walletID, address)
		return writeErr
	})
	if err != nil {
		if created, found, recoveryErr := h.committed(ctx, cmd, walletID); recoveryErr != nil || found {
			return created, recoveryErr
		}
		return CreatedCabal{}, errs.Wrap(err, errs.CodeOf(err), createCabalOp, slog.String("privy_wallet_id", walletID))
	}
	return created, nil
}

func (cmd CreateCabal) validate() error {
	if cmd.IdempotencyKey == "" {
		return errs.New(errs.CodeInvalidInput, createCabalOp, slog.String("reason", "idempotency_key"))
	}
	if _, err := domain.ParseName(cmd.Name.String()); err != nil {
		return err
	}
	_, err := domain.NewRules(
		string(cmd.Rules.JoinMode()), string(cmd.Rules.VoterMode()), string(cmd.Rules.Threshold()),
		cmd.Rules.ExpirySeconds(), cmd.Rules.SlippageBps(),
	)
	return err
}

func (h *CreateCabalHandler) committed(
	ctx context.Context, cmd CreateCabal, walletID string,
) (CreatedCabal, bool, error) {
	wallet, err := sqlc.New(h.reads).FindTreasuryWalletByPrivyWalletID(ctx, walletID)
	if errors.Is(err, sql.ErrNoRows) {
		return CreatedCabal{}, false, nil
	}
	if err != nil {
		return CreatedCabal{}, false, errs.Wrap(err, errs.CodeInternal, createCabalOp)
	}
	cabal, err := sqlc.New(h.reads).FindCabal(ctx, wallet.CabalID)
	if err != nil {
		return CreatedCabal{}, false, errs.Wrap(err, errs.CodeInternal, createCabalOp)
	}
	if cabal.CreatorID != cmd.ActorID.UUID() || cabal.Name != cmd.Name.String() ||
		cabal.JoinMode != string(cmd.Rules.JoinMode()) || cabal.VoterMode != string(cmd.Rules.VoterMode()) ||
		cabal.Threshold != string(cmd.Rules.Threshold()) ||
		cabal.ProposalExpirySeconds != cmd.Rules.ExpirySeconds() || cabal.SlippageBps != cmd.Rules.SlippageBps() {
		return CreatedCabal{}, false, errs.New(
			errs.CodeIdempotencyMismatch, createCabalOp, slog.String("privy_wallet_id", walletID),
		)
	}
	return CreatedCabal{
		ID: ids.CabalIDFrom(cabal.ID), Name: cabal.Name,
		TreasuryAddress: chain.SolanaAddress(wallet.Address), PrivyWalletID: wallet.PrivyWalletID,
	}, true, nil
}

func (h *CreateCabalHandler) write(
	ctx context.Context, tx db.Tx, cmd CreateCabal, walletID string, address chain.SolanaAddress,
) (CreatedCabal, error) {
	q := sqlc.New(tx.Queries())
	id := ids.CabalIDFrom(h.ids.NewV7())
	now := h.clock.Now()
	if err := insertCabal(ctx, q, sqlc.InsertCabalParams{
		ID: id.UUID(), Name: cmd.Name.String(), CreatorID: cmd.ActorID.UUID(),
		JoinMode: string(cmd.Rules.JoinMode()), VoterMode: string(cmd.Rules.VoterMode()),
		Threshold: string(cmd.Rules.Threshold()), ProposalExpirySeconds: cmd.Rules.ExpirySeconds(),
		SlippageBps: cmd.Rules.SlippageBps(), Now: now,
	}); err != nil {
		return CreatedCabal{}, err
	}
	n, err := q.InsertMember(ctx, sqlc.InsertMemberParams{
		CabalID: id.UUID(), UserID: cmd.ActorID.UUID(), Role: string(domain.RoleCreator),
		CanVote: domain.VoterFor(cmd.Rules, domain.RoleCreator), JoinedAt: now,
	})
	if err != nil || n != 1 {
		return CreatedCabal{}, errs.Wrap(err, errs.CodeInternal, createCabalOp, slog.Int64("member_rows", n))
	}
	if err := q.InsertTreasuryWallet(ctx, sqlc.InsertTreasuryWalletParams{
		CabalID: id.UUID(), PrivyWalletID: walletID, Address: string(address), CreatedAt: now,
	}); err != nil {
		return CreatedCabal{}, err
	}
	created := events.CabalCreated{
		V: 1, CabalID: id.UUID(), CreatorID: cmd.ActorID.UUID(), Name: cmd.Name.String(),
		JoinMode: string(cmd.Rules.JoinMode()), VoterMode: string(cmd.Rules.VoterMode()),
		Threshold: string(cmd.Rules.Threshold()), ProposalExpirySeconds: cmd.Rules.ExpirySeconds(),
		SlippageBps: cmd.Rules.SlippageBps(), TreasuryAddress: address,
	}
	joined := events.CabalMemberJoined{
		V: 1, CabalID: id.UUID(), UserID: cmd.ActorID.UUID(), Role: string(domain.RoleCreator), Via: "create",
	}
	if err := appendCreated(ctx, tx, created, joined); err != nil {
		return CreatedCabal{}, err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.CabalCreated,
			slog.String("cabal_id", id.String()), slog.String("treasury_address", string(address)))
	})
	return CreatedCabal{
		ID: id, Name: cmd.Name.String(), TreasuryAddress: address, PrivyWalletID: walletID,
	}, nil
}

func appendCreated(
	ctx context.Context, tx db.Tx, created events.CabalCreated, joined events.CabalMemberJoined,
) error {
	if err := tx.Events.Append(ctx, created); err != nil {
		return err
	}
	return tx.Events.Append(ctx, joined)
}

func insertCabal(ctx context.Context, q *sqlc.Queries, row sqlc.InsertCabalParams) error {
	n, err := q.InsertCabal(ctx, row)
	if err != nil || n != 1 {
		return errs.Wrap(err, errs.CodeInternal, createCabalOp, slog.Int64("cabal_rows", n))
	}
	return nil
}
