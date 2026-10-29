package ranking_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type stubReads struct {
	payload []byte
	err     error
}

func (s stubReads) GetLedger(context.Context, sqlc.GetLedgerParams) ([]byte, error) {
	return s.payload, s.err
}

func (stubReads) LatestLedger(context.Context, uuid.UUID) ([]byte, error) { return nil, nil }

func TestLedgerReader_Ledger(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("ledger-reader")))
	end := time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC)
	stored := domain.Ledger{Week: "2026-W42", Voted: 3, Eligible: 3, WindowEnd: end}
	for name, tc := range map[string]struct {
		reads stubReads
		want  domain.Ledger
		found bool
		code  errs.Code
	}{
		"a stored ledger":  {stubReads{payload: []byte(mustJSON(t, stored))}, stored, true, ""},
		"no stored ledger": {stubReads{err: sql.ErrNoRows}, domain.Ledger{}, false, ""},
		"a read failure":   {stubReads{err: errs.New(errs.CodeDBUnavailable, "boom")}, domain.Ledger{}, false, errs.CodeInternal},
		"a broken payload": {stubReads{payload: []byte("{")}, domain.Ledger{}, false, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, found, err := app.NewLedgerReader(tc.reads).Ledger(t.Context(), user, "2026-W42")

			if (err != nil && errs.CodeOf(err) != tc.code) || (err == nil && tc.code != "") || found != tc.found ||
				!reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Ledger = %+v, %v, %v, want %+v, %v, code %q", got, found, err, tc.want, tc.found, tc.code)
			}
		})
	}
}
