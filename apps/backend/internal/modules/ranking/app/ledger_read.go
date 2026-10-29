package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const opLedgerRead = "ranking.LedgerReader.Ledger"

type LedgerReader struct{ Reads LedgerReads }

func NewLedgerReader(reads LedgerReads) LedgerReader { return LedgerReader{Reads: reads} }

func (r LedgerReader) Ledger(ctx context.Context, user ids.UserID, week string) (domain.Ledger, bool, error) {
	payload, err := r.Reads.GetLedger(ctx, sqlc.GetLedgerParams{UserID: user.UUID(), Week: week})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ledger{}, false, nil
	}
	if err != nil {
		return domain.Ledger{}, false, errs.Wrap(err, errs.CodeInternal, opLedgerRead)
	}
	var ledger domain.Ledger
	if err := json.Unmarshal(payload, &ledger); err != nil {
		return domain.Ledger{}, false, errs.Wrap(err, errs.CodeDecodeFailed, opLedgerRead)
	}
	return ledger, true, nil
}
