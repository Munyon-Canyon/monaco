package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const swapHistoryLimit = 50

type TxnEntry struct {
	Seq     int16
	Account string
	Asset   string
	Amount  int64
}

type LedgerTxn struct {
	TxnHeader
	UserID     *ids.UserID
	SwapID     *ids.SwapID
	TransferID *uuid.UUID
	Entries    []TxnEntry
}

type SwapEvent struct {
	Type string
	At   time.Time
}

type SwapDetail struct {
	ID          ids.SwapID
	Status      string
	FailureCode string
	RequestID   string
	TxSignature string
	CreatedAt   time.Time
	History     []SwapEvent
}

type TxnView struct {
	Ledger *LedgerTxn
	Swap   *SwapDetail
}

type TxnLookup struct {
	Ledger   LedgerTxns
	Swaps    SwapReader
	Requests RequestIDs
	Events   EventHistory
}

func (v TxnView) Signature() string {
	if v.Ledger != nil && v.Ledger.TxSignature != "" {
		return v.Ledger.TxSignature
	}
	if v.Swap != nil {
		return v.Swap.TxSignature
	}
	return ""
}

func (l TxnLookup) ByID(ctx context.Context, id uuid.UUID) (TxnView, error) {
	txn, found, err := l.Ledger.TxnByID(ctx, id)
	switch {
	case err != nil:
		return TxnView{}, err
	case found:
		return l.withSwap(ctx, ledgerTxn(txn))
	}
	swap, err := l.Swaps.Swap(ctx, ids.SwapIDFrom(id))
	return l.swapOnly(ctx, swap, err)
}

func (l TxnLookup) BySignature(ctx context.Context, sig chain.Signature) (TxnView, error) {
	txn, found, err := l.Ledger.TxnBySignature(ctx, sig)
	switch {
	case err != nil:
		return TxnView{}, err
	case found:
		return l.withSwap(ctx, ledgerTxn(txn))
	}
	swap, err := l.Swaps.SwapBySignature(ctx, sig)
	return l.swapOnly(ctx, swap, err)
}

func (l TxnLookup) withSwap(ctx context.Context, txn LedgerTxn) (TxnView, error) {
	view := TxnView{Ledger: &txn}
	if txn.SwapID == nil {
		return view, nil
	}
	swap, err := l.Swaps.Swap(ctx, *txn.SwapID)
	if errs.CodeOf(err) == errs.CodeSwapNotFound {
		return view, nil
	}
	if err != nil {
		return TxnView{}, err
	}
	view.Swap, err = l.detail(ctx, swap)
	return view, err
}

func (l TxnLookup) swapOnly(ctx context.Context, swap trading.SwapView, err error) (TxnView, error) {
	switch {
	case errs.CodeOf(err) == errs.CodeSwapNotFound:
		return TxnView{}, errs.New(errs.CodeTxnNotFound, "admin.TxnLookup")
	case err != nil:
		return TxnView{}, err
	}
	detail, err := l.detail(ctx, swap)
	return TxnView{Swap: detail}, err
}

func (l TxnLookup) detail(ctx context.Context, swap trading.SwapView) (*SwapDetail, error) {
	requestID, err := l.Requests.ExecuteRequestID(ctx, swap.ID)
	if err != nil {
		return nil, err
	}
	history, err := l.Events.EventsByAggregate(ctx, "swap", swap.ID.UUID(), []string{
		string(events.TypeTradeSubmitted), string(events.TypeTradeConfirmed), string(events.TypeTradeFailed),
	}, swapHistoryLimit)
	if err != nil {
		return nil, err
	}
	detail := SwapDetail{
		ID: swap.ID, Status: string(swap.Status), FailureCode: string(swap.FailureCode), RequestID: requestID,
		TxSignature: string(swap.TxSignature), CreatedAt: swap.CreatedAt,
	}
	for _, row := range history {
		detail.History = append(detail.History, SwapEvent{Type: row.Type, At: row.CreatedAt})
	}
	return &detail, nil
}

func ledgerTxn(t treasuryport.Txn) LedgerTxn {
	out := LedgerTxn{
		TxnHeader:  txnHeader(t.TxnHeader),
		UserID:     t.UserID,
		SwapID:     t.SwapID,
		TransferID: t.TransferID,
		Entries:    make([]TxnEntry, len(t.Entries)),
	}
	for i, e := range t.Entries {
		out.Entries[i] = TxnEntry(e)
	}
	return out
}
