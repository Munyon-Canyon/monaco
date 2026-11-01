package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const resolveEVMWithdrawalOp = "funding.ResolveEVMWithdrawal"

const operatorActorPrefix = "operator:"

type ResolveEVMWithdrawal struct {
	ID       uuid.UUID
	To       domain.WithdrawalStatus
	Reason   string
	Actor    string
	Override bool
}

type EVMWithdrawalResolverDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Clock   clock.Clock
	Chain   SignatureStatuses
	Wallets SigningWallets
	Actions multichain.WalletActions
	Hints   HintPublisher
}

type EVMWithdrawalResolver struct{ d EVMWithdrawalResolverDeps }

func NewEVMWithdrawalResolver(d EVMWithdrawalResolverDeps) *EVMWithdrawalResolver {
	return &EVMWithdrawalResolver{d: d}
}

type EVMStepFact struct {
	Network chain.Network        `json:"network"`
	TxHash  string               `json:"tx_hash,omitempty"`
	State   multichain.StepState `json:"state"`
}

type EVMWithdrawalFacts struct {
	Action          multichain.ActionState `json:"privy_action_state"`
	Steps           []EVMStepFact          `json:"steps"`
	SolanaSignature chain.Signature        `json:"solana_signature,omitempty"`
	SolanaChain     string                 `json:"solana_chain_state,omitempty"`
	Contradiction   string                 `json:"contradiction,omitempty"`
	Unverified      string                 `json:"unverified,omitempty"`
}

func (f EVMWithdrawalFacts) stepSummary() string {
	parts := make([]string, len(f.Steps))
	for i, step := range f.Steps {
		parts[i] = string(step.Network) + ":" + string(step.State)
	}
	return strings.Join(parts, ",")
}

type EVMWithdrawalResolution struct {
	ID     uuid.UUID
	Status domain.WithdrawalStatus
	Facts  EVMWithdrawalFacts
}

func (h *EVMWithdrawalResolver) Handle(ctx context.Context, cmd ResolveEVMWithdrawal) (EVMWithdrawalResolution, error) {
	if err := cmd.validate(); err != nil {
		return EVMWithdrawalResolution{}, err
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	row, err := sqlc.New(h.d.Reads).EVMWithdrawalForResolve(ctx, cmd.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return EVMWithdrawalResolution{}, errs.Wrap(err, errs.CodeNotFound, resolveEVMWithdrawalOp)
	}
	if err != nil {
		return EVMWithdrawalResolution{}, errs.Wrap(err, errs.CodeInternal, resolveEVMWithdrawalOp)
	}
	if domain.WithdrawalStatus(row.Status) != domain.WithdrawalSubmitted {
		return h.settled(row, cmd)
	}
	facts, err := h.verify(ctx, cmd, row)
	if err != nil {
		return EVMWithdrawalResolution{}, err
	}
	return h.resolve(ctx, cmd, row, facts)
}

func (h *EVMWithdrawalResolver) settled(
	row sqlc.EVMWithdrawalForResolveRow, cmd ResolveEVMWithdrawal,
) (EVMWithdrawalResolution, error) {
	if domain.WithdrawalStatus(row.Status) == cmd.To {
		return EVMWithdrawalResolution{ID: row.ID, Status: cmd.To}, nil
	}
	return EVMWithdrawalResolution{}, errs.New(errs.CodeVersionConflict, resolveEVMWithdrawalOp,
		slog.String("withdrawal_id", row.ID.String()), slog.String("status", row.Status))
}

func (h *EVMWithdrawalResolver) verify(
	ctx context.Context, cmd ResolveEVMWithdrawal, row sqlc.EVMWithdrawalForResolveRow,
) (EVMWithdrawalFacts, error) {
	facts, err := h.read(ctx, cmd, row)
	switch {
	case err == nil && facts.Contradiction != "" && !cmd.Override:
		return EVMWithdrawalFacts{}, fmt.Errorf(
			"%w: the chain contradicts --to %s: %s; rerun with --override to book it anyway",
			errs.New(errs.CodeInvalidInput, resolveEVMWithdrawalOp), cmd.To, facts.Contradiction,
		)
	case err != nil && cmd.Override:
		return EVMWithdrawalFacts{Unverified: err.Error()}, nil
	}
	return facts, err
}

func (h *EVMWithdrawalResolver) read(
	ctx context.Context, cmd ResolveEVMWithdrawal, row sqlc.EVMWithdrawalForResolveRow,
) (EVMWithdrawalFacts, error) {
	wallet, err := h.d.Wallets.SigningWallet(ctx, ids.UserIDFrom(row.UserID))
	if err != nil {
		return EVMWithdrawalFacts{}, err
	}
	status, err := h.d.Actions.Action(ctx, wallet.ID, multichain.ActionID(row.PrivyActionID))
	if err != nil {
		return EVMWithdrawalFacts{}, err
	}
	facts := EVMWithdrawalFacts{Action: status.State, Steps: make([]EVMStepFact, len(status.Steps))}
	for i, step := range status.Steps {
		facts.Steps[i] = EVMStepFact{Network: step.Network, TxHash: step.TxHash, State: step.State}
	}
	facts.SolanaSignature = solanaSignature(row, status)
	if facts.SolanaSignature != "" {
		statuses, err := h.d.Chain.SignatureStatuses(ctx, []chain.Signature{facts.SolanaSignature})
		if err != nil {
			return EVMWithdrawalFacts{}, err
		}
		if len(statuses) != 1 {
			return EVMWithdrawalFacts{}, errs.New(errs.CodeDecodeFailed, resolveEVMWithdrawalOp,
				slog.Int("statuses", len(statuses)))
		}
		facts.SolanaChain = solanaChainState(statuses[0])
	}
	facts.Contradiction = contradiction(cmd.To, facts)
	return facts, nil
}

const (
	chainFinalized  = "finalized"
	chainFailed     = "failed"
	chainProcessing = "processing"
	chainNotFound   = "not_found"
)

func solanaSignature(row sqlc.EVMWithdrawalForResolveRow, status multichain.ActionStatus) chain.Signature {
	if row.TxSignature != "" {
		return chain.Signature(row.TxSignature)
	}
	for _, step := range status.Steps {
		if step.Network == chain.NetworkSolana && step.TxHash != "" {
			return chain.Signature(step.TxHash)
		}
	}
	return ""
}

func solanaChainState(s solana.Status) string {
	switch {
	case s.State == solana.StateFinalized && s.Failed:
		return chainFailed
	case s.State == solana.StateFinalized:
		return chainFinalized
	case s.State == solana.StateNotFound:
		return chainNotFound
	}
	return chainProcessing
}

func contradiction(to domain.WithdrawalStatus, f EVMWithdrawalFacts) string {
	var evmLanded, evmMayLand, solConfirmed bool
	for _, step := range f.Steps {
		switch {
		case step.Network == chain.NetworkSolana:
			solConfirmed = solConfirmed || step.State == multichain.StepConfirmed
		case step.State == multichain.StepConfirmed:
			evmLanded = true
		case step.TxHash != "" && step.State == multichain.StepPending:
			evmMayLand = true
		}
	}
	if to == domain.WithdrawalFailed {
		return failedContradiction(f, evmLanded, evmMayLand, solConfirmed)
	}
	switch {
	case f.SolanaSignature == "":
		return "no Solana signature is recorded, so the withdrawal cannot be booked"
	case f.SolanaChain == chainFailed:
		return "its Solana step failed on chain"
	case !evmLanded && f.Action != multichain.ActionSucceeded:
		return "no EVM step shows the funds arrived"
	}
	return ""
}

func failedContradiction(f EVMWithdrawalFacts, evmLanded, evmMayLand, solConfirmed bool) string {
	switch {
	case f.Action == multichain.ActionSucceeded:
		return "Privy reports the action succeeded"
	case evmLanded:
		return "an EVM step landed"
	case evmMayLand:
		return "an EVM step was broadcast and may yet land"
	case solConfirmed || f.SolanaChain == chainFinalized:
		return "its Solana step landed, so the funds left the member's wallet"
	case f.SolanaChain == chainProcessing:
		return "its Solana step is still processing and may yet land"
	case f.SolanaChain == chainNotFound:
		return "its Solana step was broadcast and the chain has not shown it failed or expired"
	}
	return ""
}

func (h *EVMWithdrawalResolver) resolve(
	ctx context.Context, cmd ResolveEVMWithdrawal, row sqlc.EVMWithdrawalForResolveRow, facts EVMWithdrawalFacts,
) (EVMWithdrawalResolution, error) {
	amount, err := money.ParseMicros(row.AmountMicros)
	if err != nil {
		return EVMWithdrawalResolution{}, errs.Wrap(err, errs.CodeDecodeFailed, resolveEVMWithdrawalOp)
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := h.apply(ctx, tx, cmd, row, amount, facts)
		if err == nil && !moved {
			err = errs.New(errs.CodeVersionConflict, resolveEVMWithdrawalOp,
				slog.String("withdrawal_id", row.ID.String()))
		}
		return err
	})
	if err != nil {
		return EVMWithdrawalResolution{}, err
	}
	return EVMWithdrawalResolution{ID: row.ID, Status: cmd.To, Facts: facts}, nil
}

func (h *EVMWithdrawalResolver) apply(
	ctx context.Context, tx db.Tx, cmd ResolveEVMWithdrawal, row sqlc.EVMWithdrawalForResolveRow,
	amount money.Micros, facts EVMWithdrawalFacts,
) (bool, error) {
	q := sqlc.New(tx.Queries())
	now := h.d.Clock.Now()
	var n int64
	var err error
	var event events.Event
	if cmd.To == domain.WithdrawalConfirmed {
		if _, err = q.SetWithdrawalSourceSignature(ctx, sqlc.SetWithdrawalSourceSignatureParams{
			ID: row.ID, TxSignature: string(facts.SolanaSignature),
		}); err != nil {
			return false, err
		}
		n, err = q.ConfirmWithdrawal(ctx, sqlc.ConfirmWithdrawalParams{ID: row.ID, CompletedAt: now})
		event = events.WithdrawalConfirmed{
			V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount,
			ToAddress: chain.SolanaAddress(row.EvmAddress), TxSignature: facts.SolanaSignature,
		}
	} else {
		n, err = q.FailWithdrawal(ctx, sqlc.FailWithdrawalParams{
			ID: row.ID, FailCode: domain.WithdrawalSupportResolved, CompletedAt: now,
		})
		event = events.WithdrawalFailed{
			V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount,
			Code: domain.WithdrawalSupportResolved,
		}
	}
	if err != nil || n == 0 {
		return false, err
	}
	user := ids.UserIDFrom(row.UserID)
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(user), nil)
		observability.Info(
			ctx,
			observability.FundingWithdrawalSupportResolved,
			slog.String("withdrawal_id", row.ID.String()),
			slog.String("user_id", row.UserID.String()),
			slog.String("amount_micros", amount.String()),
			slog.String(
				"status_before",
				string(domain.WithdrawalSubmitted),
			),
			slog.String("status_after", string(cmd.To)),
			slog.String("actor", cmd.Actor),
			slog.String("reason", cmd.Reason),
			slog.Bool("override", cmd.Override),
			slog.String("action_state", string(facts.Action)),
			slog.String("steps", facts.stepSummary()),
			slog.String("solana_chain_state", facts.SolanaChain),
			slog.String("contradiction", facts.Contradiction),
			slog.String("unverified", facts.Unverified),
		)
	})
	return true, tx.Events.Append(ctx, event)
}

func (cmd ResolveEVMWithdrawal) validate() error {
	validActor := strings.HasPrefix(cmd.Actor, operatorActorPrefix) && len(cmd.Actor) > len(operatorActorPrefix)
	reason := utf8.RuneCountInString(cmd.Reason)
	validTo := cmd.To == domain.WithdrawalConfirmed || cmd.To == domain.WithdrawalFailed
	if cmd.ID == uuid.Nil || !validActor || reason < 1 || reason > 500 || !validTo {
		return errs.New(errs.CodeInvalidInput, resolveEVMWithdrawalOp)
	}
	return nil
}
