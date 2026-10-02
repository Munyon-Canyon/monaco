package app

import (
	"context"
	"crypto/rand"
	"io"
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
	InviteCode      string
	TreasuryAddress chain.SolanaAddress
	PrivyWalletID   string
}

type CreateCabalDeps struct {
	UoW     *db.UnitOfWork
	Wallets TreasuryWallets
	IDs     ids.Generator
	Clock   clock.Clock
	Random  io.Reader
}

type CreateCabalHandler struct {
	uow     *db.UnitOfWork
	wallets TreasuryWallets
	ids     ids.Generator
	clock   clock.Clock
	random  io.Reader
}

func NewCreateCabalHandler(d CreateCabalDeps) *CreateCabalHandler {
	if d.Random == nil {
		d.Random = rand.Reader
	}
	return &CreateCabalHandler{uow: d.UoW, wallets: d.Wallets, ids: d.IDs, clock: d.Clock, random: d.Random}
}

func TreasuryKey(actor ids.UserID, idempotencyKey string) string {
	return "cabal-treasury:" + actor.String() + ":" + idempotencyKey
}

func (h *CreateCabalHandler) Handle(ctx context.Context, cmd CreateCabal) (CreatedCabal, error) {
	if cmd.IdempotencyKey == "" {
		return CreatedCabal{}, errs.New(errs.CodeInvalidInput, createCabalOp, slog.String("reason", "idempotency_key"))
	}
	walletID, address, err := h.wallets.CreateAppOwned(ctx, TreasuryKey(cmd.ActorID, cmd.IdempotencyKey))
	if err != nil {
		return CreatedCabal{}, err
	}
	var created CreatedCabal
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var writeErr error
		created, writeErr = h.write(ctx, tx, cmd, walletID, address)
		return writeErr
	})
	if err != nil {
		return CreatedCabal{}, errs.Wrap(err, errs.CodeOf(err), createCabalOp, slog.String("privy_wallet_id", walletID))
	}
	return created, nil
}

func (h *CreateCabalHandler) write(
	ctx context.Context, tx db.Tx, cmd CreateCabal, walletID string, address chain.SolanaAddress,
) (CreatedCabal, error) {
	q := sqlc.New(tx.Queries())
	id := ids.CabalIDFrom(h.ids.NewV7())
	now := h.clock.Now()
	code, err := insertCabal(ctx, q, sqlc.InsertCabalParams{
		ID: id.UUID(), Name: cmd.Name.String(), CreatorID: cmd.ActorID.UUID(),
		JoinMode: string(cmd.Rules.JoinMode()), VoterMode: string(cmd.Rules.VoterMode()),
		Threshold: string(cmd.Rules.Threshold()), ProposalExpirySeconds: cmd.Rules.ExpirySeconds(),
		SlippageBps: cmd.Rules.SlippageBps(), Now: now,
	}, h.random)
	if err != nil {
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
	return CreatedCabal{
		ID: id, Name: cmd.Name.String(), InviteCode: code, TreasuryAddress: address, PrivyWalletID: walletID,
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

func insertCabal(
	ctx context.Context, q *sqlc.Queries, row sqlc.InsertCabalParams, random io.Reader,
) (string, error) {
	code, err := domain.NewInviteCode(random)
	if err != nil {
		return "", err
	}
	row.InviteCode = code.String()
	n, err := q.InsertCabal(ctx, row)
	if err != nil || n == 1 {
		return row.InviteCode, err
	}
	code, err = domain.NewInviteCode(random)
	if err != nil {
		return "", err
	}
	row.InviteCode = code.String()
	n, err = q.InsertCabal(ctx, row)
	if err != nil || n != 1 {
		return "", errs.Wrap(err, errs.CodeInternal, createCabalOp, slog.Int64("cabal_rows", n))
	}
	return row.InviteCode, nil
}
