package adapters

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
)

const explorerTxURL = "https://solscan.io/tx/"

func (h HTTP) GetAdminTxn(
	ctx context.Context, req api.GetAdminTxnRequestObject,
) (api.GetAdminTxnResponseObject, error) {
	view, err := h.Txns.ByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetAdminTxn200JSONResponse(txnBody(view)), nil
}

func (h HTTP) FindAdminTxn(
	ctx context.Context, req api.FindAdminTxnRequestObject,
) (api.FindAdminTxnResponseObject, error) {
	view, err := h.Txns.BySignature(ctx, chain.Signature(req.Params.Signature))
	if err != nil {
		return nil, err
	}
	return api.FindAdminTxn200JSONResponse(txnBody(view)), nil
}

func txnBody(v app.TxnView) api.AdminTxn {
	body := api.AdminTxn{}
	if v.Ledger != nil {
		ledger := ledgerBody(*v.Ledger)
		body.Ledger = &ledger
	}
	if v.Swap != nil {
		swap := swapBody(*v.Swap)
		body.Swap = &swap
	}
	if sig := v.Signature(); sig != "" {
		url := explorerTxURL + sig
		body.ExplorerUrl = &url
	}
	return body
}

func ledgerBody(t app.LedgerTxn) api.AdminLedgerTxn {
	entries := make([]api.AdminTxnEntry, len(t.Entries))
	for i, e := range t.Entries {
		entries[i] = api.AdminTxnEntry{
			Seq: int32(e.Seq), Account: e.Account, Asset: e.Asset, Amount: strconv.FormatInt(e.Amount, 10),
		}
	}
	header := txnHeader(t.TxnHeader)
	return api.AdminLedgerTxn{
		Id: header.Id, Scope: header.Scope, UserId: uuidOf(t.UserID), CabalId: header.CabalId,
		Kind: header.Kind, Status: header.Status, SwapId: uuidOf(t.SwapID),
		TransferId: t.TransferID, TxSignature: header.TxSignature, CreatedAt: header.CreatedAt, Entries: entries,
	}
}

func swapBody(s app.SwapDetail) api.AdminSwap {
	history := make([]api.AdminSwapEvent, len(s.History))
	for i, e := range s.History {
		history[i] = api.AdminSwapEvent{Type: e.Type, At: e.At.UTC()}
	}
	return api.AdminSwap{
		Id: s.ID.UUID(), Status: s.Status, FailureCode: optionalString(s.FailureCode),
		ExecuteRequestId: optionalString(s.RequestID), TxSignature: optionalString(s.TxSignature),
		CreatedAt: s.CreatedAt.UTC(), StatusHistory: history,
	}
}

func uuidOf[T interface{ UUID() uuid.UUID }](id *T) *uuid.UUID {
	if id == nil {
		return nil
	}
	u := (*id).UUID()
	return &u
}
