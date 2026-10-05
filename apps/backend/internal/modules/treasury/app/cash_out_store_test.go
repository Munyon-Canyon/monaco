package app

import (
	"context"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type cashOutTestQueries struct {
	shares                                        sqlc.CashOutSharesRow
	job                                           sqlc.CashOutJobByIDRow
	shareErr, jobErr, liveErr, cashErr, insertErr error
	live                                          bool
	cash                                          string
}

func (q cashOutTestQueries) CashOutJobByID(context.Context, sqlc.CashOutJobByIDParams) (sqlc.CashOutJobByIDRow, error) {
	return q.job, q.jobErr
}

func (q cashOutTestQueries) CashOutLiveJob(context.Context, sqlc.CashOutLiveJobParams) (bool, error) {
	return q.live, q.liveErr
}

func (q cashOutTestQueries) CashOutShares(context.Context, sqlc.CashOutSharesParams) (sqlc.CashOutSharesRow, error) {
	return q.shares, q.shareErr
}

func (q cashOutTestQueries) CashOutShortfall(context.Context, sqlc.CashOutShortfallParams) (string, error) {
	return q.cash, q.cashErr
}

func (q cashOutTestQueries) InsertCashOutJob(context.Context, sqlc.InsertCashOutJobParams) error {
	return q.insertErr
}

type cashOutTestValues struct {
	port.PositionsReader
	err error
}

func (v cashOutTestValues) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	return money.MicrosFromUint64(1_000_000), v.err
}

func TestCashOut_queriesFail(t *testing.T) {
	t.Parallel()
	testErr := errs.New(errs.CodeInternal, "test")
	q := cashOutTestQueries{shares: sqlc.CashOutSharesRow{Shares: "100", Total: "100"}, cash: "0"}
	h := CashOutHandler{
		values: cashOutTestValues{}, usdc: "USDC", clock: clock.Real{}, ids: ids.Real{},
		post: func(context.Context, db.Tx, domain.UserTxn) error { return testErr },
	}
	cmd := CashOut{PayoutMicros: money.MicrosFromUint64(100_000)}
	units, payout := money.SharesUnitsFromUint64(10), cmd.PayoutMicros
	amount := func() {
		if _, _, err := h.amount(t.Context(), q, cmd); err == nil {
			t.Fatal("amount error = nil")
		}
	}
	record := func() {
		if _, err := h.record(t.Context(), db.Tx{}, q, cmd, units, payout); err == nil {
			t.Fatal("record error = nil")
		}
	}
	q.shareErr = testErr
	amount()
	q.shareErr = nil
	q.shares.Shares = "bad"
	amount()
	q.shares.Shares = "100"
	q.shares.Total = "bad"
	amount()
	q.shares.Total = "100"
	q.shares.Total = "0"
	amount()
	q.shares.Total = "100"
	q.liveErr = testErr
	amount()
	q.liveErr = nil
	h.values = cashOutTestValues{err: testErr}
	amount()
	h.values = cashOutTestValues{}
	q.cashErr = testErr
	record()
	q.cashErr = nil
	q.cash = "bad"
	record()
	q.cash = "0"
	q.insertErr = testErr
	record()
	q.insertErr = nil
	record()
	q.cash = "0"
	payout = money.MicrosFromUint64(math.MaxUint64)
	record()
	payout, units = cmd.PayoutMicros, money.SharesUnitsFromUint64(math.MaxUint64)
	record()
	units = money.SharesUnits{}
	record()
}

func TestCashOut_readsFail(t *testing.T) {
	t.Parallel()
	errTest := errs.New(errs.CodeInternal, "test")
	pause := CashOutPauseFunc(func(context.Context, ids.CabalID) (CashOutPause, error) { return CashOutPause{}, nil })
	h := func(q cashOutTestQueries, v cashOutTestValues, p CashOutPauses) *CashOutHandler {
		return &CashOutHandler{reads: q, values: v, pauses: p}
	}
	for _, q := range []cashOutTestQueries{
		{shareErr: errTest},
		{shares: sqlc.CashOutSharesRow{Shares: "bad", Total: "1"}},
		{shares: sqlc.CashOutSharesRow{Shares: "1", Total: "bad"}},
		{shares: sqlc.CashOutSharesRow{Shares: "1", Total: "0"}},
	} {
		if _, err := h(q, cashOutTestValues{}, pause).Preview(t.Context(), ids.CabalID{}, ids.UserID{}); err == nil {
			t.Fatal("Preview error = nil")
		}
	}
	pausedErr := CashOutPauseFunc(func(context.Context, ids.CabalID) (CashOutPause, error) {
		return CashOutPause{}, errTest
	})
	if _, err := h(cashOutTestQueries{}, cashOutTestValues{}, pausedErr).
		Preview(t.Context(), ids.CabalID{}, ids.UserID{}); err == nil {
		t.Fatal("paused Preview error = nil")
	}
	if _, err := h(
		cashOutTestQueries{shares: sqlc.CashOutSharesRow{Shares: "1", Total: "1"}},
		cashOutTestValues{err: errTest},
		pause,
	).Preview(t.Context(), ids.CabalID{}, ids.UserID{}); err == nil {
		t.Fatal("value Preview error = nil")
	}
	preview, err := h(
		cashOutTestQueries{shares: sqlc.CashOutSharesRow{Shares: "0"}}, cashOutTestValues{}, pause,
	).Preview(t.Context(), ids.CabalID{}, ids.UserID{})
	if err != nil || !preview.ShareUnits.IsZero() {
		t.Fatalf("zero Preview = %#v, %v", preview, err)
	}
	for _, q := range []cashOutTestQueries{
		{jobErr: errTest},
		{job: sqlc.CashOutJobByIDRow{ShareUnits: "bad"}},
		{job: sqlc.CashOutJobByIDRow{ShareUnits: "1", PayoutMicros: "bad"}},
		{job: sqlc.CashOutJobByIDRow{ShareUnits: "1", PayoutMicros: "1", SellUsdcMicros: "bad"}},
		{job: sqlc.CashOutJobByIDRow{ShareUnits: "1", PayoutMicros: "1", SellUsdcMicros: "0", Status: "bad"}},
	} {
		if _, err := h(q, cashOutTestValues{}, pause).
			Job(t.Context(), ids.CabalID{}, ids.UserID{}, q.job.ID); err == nil {
			t.Fatal("Job error = nil")
		}
	}
}

func TestCashOut_startReturnsLockFailure(t *testing.T) {
	t.Parallel()
	errTest := errs.New(errs.CodeInternal, "test")
	h := CashOutHandler{locker: func(context.Context, db.Tx, ids.CabalID) error { return errTest }}
	if _, err := h.start(t.Context(), db.Tx{}, CashOut{}); err == nil {
		t.Fatal("start error = nil")
	}
}
