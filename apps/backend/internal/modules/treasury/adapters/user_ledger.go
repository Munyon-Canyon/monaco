package adapters

import (
	"context"
	"encoding/json"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type UserLedger struct {
	Ledger app.Ledger
	IDs    ids.Generator
	USDC   domain.Asset
}

func (h UserLedger) Handle(ctx context.Context, tx db.Tx, e events.DepositCredited, _ time.Time) error {
	amount, err := e.AmountMicros.Delta(money.Micros{})
	if err != nil {
		return err
	}
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: e.DepositID, UserID: ids.UserIDFrom(e.UserID), Kind: domain.UserDeposit,
		Status: domain.TxnSettled, TxSignature: e.TxSignature,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: h.USDC, Amount: amount},
		{
			Account: domain.UserExternal, Asset: h.USDC,
			Amount: money.SignedMicrosFromInt64(-amount.Int64()),
		},
	})
	if err != nil {
		return err
	}
	return h.Ledger.PostUserTxn(ctx, tx, txn)
}

func DepositCreditedBalances(usdc domain.Asset) BalanceRule {
	return func(payload []byte) ([]Balance, error) {
		var e events.DepositCredited
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "treasury.DepositCreditedBalances")
		}
		amount, err := e.AmountMicros.Delta(money.Micros{})
		if err != nil {
			return nil, err
		}
		return []Balance{{Owner: "wallet:" + e.UserID.String(), Asset: string(usdc), Amount: amount}}, nil
	}
}
