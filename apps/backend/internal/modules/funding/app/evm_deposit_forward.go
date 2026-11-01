package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	EVMDepositForwardStuckAfter = 30 * time.Minute
	EVMDepositArrivalWindow     = 24 * time.Hour
	EVMDepositWalletPage        = 100
	evmDepositExecutionPages    = 50
	evmDepositStuckBatch        = 100
)

const (
	forwardPending = "pending"
	forwardFailed  = "failed"
	forwardArrived = "arrived"
	forwardAbsent  = "absent"
)

type EVMDepositWallet struct {
	UserID   ids.UserID
	WalletID string
}

type EVMDepositWallets interface {
	EVMDepositWallets(ctx context.Context, after ids.UserID, limit int) ([]EVMDepositWallet, error)
}

type ExecutionFeed interface {
	Executions(ctx context.Context, walletID, cursor string) (multichain.ExecutionPage, error)
	Reindex(ctx context.Context, walletID string, network chain.Network) error
}

type EVMDepositForwardDeps struct {
	UoW      *db.UnitOfWork
	Reads    sqlc.DBTX
	IDs      ids.Generator
	Clock    clock.Clock
	Wallets  EVMDepositWallets
	Feed     ExecutionFeed
	Interval time.Duration
	PageSize int
}

type EVMDepositForwardPoller struct {
	d         EVMDepositForwardDeps
	mu        sync.Mutex
	alerted   map[uuid.UUID]bool
	reindexed map[string]bool
}

func NewEVMDepositForwardPoller(d EVMDepositForwardDeps) *EVMDepositForwardPoller {
	return &EVMDepositForwardPoller{d: d, alerted: map[uuid.UUID]bool{}, reindexed: map[string]bool{}}
}

func (*EVMDepositForwardPoller) Name() string { return "funding.evm_deposit_forward" }

func (p *EVMDepositForwardPoller) Interval() time.Duration { return p.d.Interval }

type forwardChange struct {
	id     uuid.UUID
	user   ids.UserID
	amount money.Micros
	from   string
	to     string
}

func (p *EVMDepositForwardPoller) Tick(ctx context.Context) (poller.Report, error) {
	var report poller.Report
	var failures []error
	after := ids.UserID{}
	for {
		wallets, err := p.d.Wallets.EVMDepositWallets(ctx, after, p.d.PageSize)
		if err != nil {
			failures = append(failures, errs.Wrap(err, errs.CodeOf(err), "funding.EVMDepositForward.wallets"))
			break
		}
		for _, w := range wallets {
			scanned, changed, err := p.syncWallet(ctx, w)
			report.Scanned += scanned
			report.Changed += changed
			failures = append(failures, err)
		}
		if len(wallets) < p.d.PageSize {
			break
		}
		after = wallets[len(wallets)-1].UserID
	}
	failures = append(failures, p.alertStuck(ctx))
	return report, errors.Join(failures...)
}

func (p *EVMDepositForwardPoller) syncWallet(ctx context.Context, w EVMDepositWallet) (int, int, error) {
	scanned, changed := 0, 0
	var failures []error
	cursor := ""
	for range evmDepositExecutionPages {
		page, err := p.d.Feed.Executions(ctx, w.WalletID, cursor)
		if err != nil {
			failures = append(failures, errs.Wrap(err, errs.CodeOf(err), "funding.EVMDepositForward.executions",
				slog.String("wallet_id", w.WalletID)))
			break
		}
		for _, ex := range page.Executions {
			scanned++
			moved, err := p.record(ctx, w, ex)
			changed += moved
			failures = append(failures, err)
		}
		if page.Next == "" || page.Next == cursor {
			break
		}
		cursor = page.Next
	}
	return scanned, changed, errors.Join(failures...)
}

func (p *EVMDepositForwardPoller) record(
	ctx context.Context, w EVMDepositWallet, ex multichain.Execution,
) (int, error) {
	const op = "funding.EVMDepositForward.record"
	if err := checkExecution(ex); err != nil {
		return 0, err
	}
	if isUnrouted(ex) {
		if err := p.reindexUnrouted(ctx, w, ex); err != nil {
			return 0, err
		}
		if ex.AmountMicros.IsZero() {
			return 0, nil
		}
	}
	var change forwardChange
	err := p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		change, err = p.apply(ctx, tx, w, ex)
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.String("execution_id", ex.ID))
	}
	if change.from == change.to {
		return 0, nil
	}
	observability.Info(ctx, observability.FundingEVMForwardMoved,
		slog.String("forward_id", change.id.String()), slog.String("user_id", change.user.String()),
		slog.String("amount_micros", change.amount.String()),
		slog.String("status_before", change.from), slog.String("status_after", change.to))
	return 1, nil
}

func isUnrouted(ex multichain.Execution) bool {
	return ex.State == multichain.ActionFailed && ex.ActionID == ""
}

func checkExecution(ex multichain.Execution) error {
	const op = "funding.EVMDepositForward.execution"
	switch {
	case ex.ID == "":
		return errs.New(errs.CodeDecodeFailed, op, slog.String("reason", "missing execution id"))
	case !ex.Network.IsEVM() || ex.Network.Cash().Symbol != "USDC":
		return errs.New(errs.CodeDecodeFailed, op, slog.String("execution_id", ex.ID),
			slog.String("network", string(ex.Network)))
	case ex.AmountMicros.IsZero() && !isUnrouted(ex):
		return errs.New(errs.CodeDecodeFailed, op, slog.String("execution_id", ex.ID),
			slog.String("reason", "zero amount"))
	}
	switch ex.State {
	case multichain.ActionPending, multichain.ActionSucceeded, multichain.ActionFailed, multichain.ActionRejected:
		return nil
	}
	return errs.New(errs.CodeDecodeFailed, op, slog.String("execution_id", ex.ID),
		slog.String("state", string(ex.State)))
}

func (p *EVMDepositForwardPoller) apply(
	ctx context.Context, tx db.Tx, w EVMDepositWallet, ex multichain.Execution,
) (forwardChange, error) {
	q := sqlc.New(tx.Queries())
	row, err := q.UpsertEVMDepositForward(ctx, sqlc.UpsertEVMDepositForwardParams{
		ID: p.d.IDs.NewV7(), UserID: w.UserID.UUID(), Network: string(ex.Network), SourceTx: ex.SourceTx,
		PrivyExecutionID: ex.ID, PrivyActionID: string(ex.ActionID),
		AmountMicros: ex.AmountMicros.String(), CreatedAt: p.d.Clock.Now(),
	})
	if err != nil {
		return forwardChange{}, err
	}
	if row.UserID != w.UserID.UUID() {
		return forwardChange{}, errs.New(errs.CodeInternal, "funding.EVMDepositForward.owner",
			slog.String("forward_id", row.ID.String()))
	}
	change := forwardChange{id: row.ID, user: w.UserID, amount: ex.AmountMicros, from: row.Status}
	if row.Inserted {
		change.from = forwardAbsent
	}
	change.to, err = follow(ctx, tx, q, row, ex)
	return change, err
}

func follow(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, row sqlc.UpsertEVMDepositForwardRow,
	ex multichain.Execution,
) (string, error) {
	if ex.State == multichain.ActionFailed || ex.State == multichain.ActionRejected {
		failed, err := q.MarkEVMDepositForwardFailed(ctx, row.ID)
		if err != nil || failed == 0 {
			return row.Status, err
		}
		return forwardFailed, nil
	}
	if row.Status == forwardFailed {
		return row.Status, nil
	}
	if _, err := q.RecordEVMDepositForwardProgress(ctx, sqlc.RecordEVMDepositForwardProgressParams{
		ID: row.ID, PrivyActionID: string(ex.ActionID), SolanaSignature: ex.Signature,
		AmountMicros: ex.AmountMicros.String(), FeeMicros: forwardFee(ex),
	}); err != nil || ex.State != multichain.ActionSucceeded {
		return row.Status, err
	}
	var arrived int64
	err := announce(ctx, tx, q, row, ex)
	if err == nil {
		arrived, err = q.ArriveEVMDepositForwardFromCredit(ctx, sqlc.ArriveEVMDepositForwardFromCreditParams{
			ID: row.ID, CreditedAfter: creditedAfter(row), WindowSeconds: EVMDepositArrivalWindow.Seconds(),
		})
	}
	if err != nil || arrived == 0 {
		return row.Status, err
	}
	return forwardArrived, nil
}

func forwardFee(ex multichain.Execution) string {
	if ex.State != multichain.ActionSucceeded || ex.InputMicros.IsZero() {
		return ""
	}
	fee, err := ex.InputMicros.Sub(ex.AmountMicros)
	if err != nil {
		return money.Micros{}.String()
	}
	return fee.String()
}

func creditedAfter(row sqlc.UpsertEVMDepositForwardRow) time.Time {
	if row.Inserted {
		return row.CreatedAt.Add(-EVMDepositArrivalWindow)
	}
	return row.CreatedAt
}

func announce(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, row sqlc.UpsertEVMDepositForwardRow,
	ex multichain.Execution,
) error {
	announced, err := q.EVMDepositForwardAnnounced(ctx, row.ID)
	if err != nil || announced {
		return err
	}
	return tx.Events.Append(ctx, events.DepositEVMForwarded{
		V: 1, ForwardID: row.ID, UserID: row.UserID, Network: ex.Network, SourceTx: ex.SourceTx,
		PrivyExecutionID: ex.ID, AmountMicros: ex.AmountMicros,
	})
}

func (p *EVMDepositForwardPoller) reindexUnrouted(
	ctx context.Context, w EVMDepositWallet, ex multichain.Execution,
) error {
	status, err := sqlc.New(p.d.Reads).EVMDepositForwardStatusByExecution(ctx, ex.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return errs.Wrap(err, errs.CodeInternal, "funding.EVMDepositForward.status")
	case status != forwardPending:
		return nil
	}
	if p.reindexedOnce(ex.ID) {
		return nil
	}
	reindexErr := p.d.Feed.Reindex(ctx, w.WalletID, ex.Network)
	observability.Alert(ctx, observability.FundingEVMForwardUnrouted,
		slog.String("execution_id", ex.ID), slog.String("wallet_id", w.WalletID),
		slog.String("network", string(ex.Network)), slog.String("code", string(errs.CodeOf(reindexErr))))
	if reindexErr != nil {
		return errs.Wrap(reindexErr, errs.CodeOf(reindexErr), "funding.EVMDepositForward.reindex")
	}
	p.markReindexed(ex.ID)
	return nil
}

func (p *EVMDepositForwardPoller) reindexedOnce(executionID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reindexed[executionID]
}

func (p *EVMDepositForwardPoller) markReindexed(executionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reindexed[executionID] = true
}

func (p *EVMDepositForwardPoller) alertStuck(ctx context.Context) error {
	rows, err := sqlc.New(p.d.Reads).ListStuckEVMDepositForwards(ctx, sqlc.ListStuckEVMDepositForwardsParams{
		CreatedBefore: p.d.Clock.Now().Add(-EVMDepositForwardStuckAfter), MaxRows: evmDepositStuckBatch,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "funding.EVMDepositForward.stuck")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	still := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		still[row.ID] = true
		if p.alerted[row.ID] {
			continue
		}
		observability.Alert(ctx, observability.FundingEVMForwardStuck,
			slog.String("forward_id", row.ID.String()), slog.String("user_id", row.UserID.String()),
			slog.String("network", row.Network), slog.Time("pending_since", row.CreatedAt))
	}
	p.alerted = still
	return nil
}
