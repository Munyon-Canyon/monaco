package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type fakeCallStore struct {
	records    map[ids.UserID]port.CallRecord
	pending    map[ids.UserID][]string
	recordsErr error
	pendingErr error
	calls      int
}

func (f *fakeCallStore) UserRecords(
	context.Context, []ids.UserID, *ids.CabalID, port.Period,
) (map[ids.UserID]port.CallRecord, error) {
	f.calls++
	return f.records, f.recordsErr
}

func (f *fakeCallStore) PendingSymbols(
	context.Context, []ids.UserID, *ids.CabalID, port.Period,
) (map[ids.UserID][]string, error) {
	return f.pending, f.pendingErr
}

func (f *fakeCallStore) CabalRecords(
	context.Context, []ids.CabalID, port.Period,
) (map[ids.CabalID]port.CallRecord, error) {
	f.calls++
	return map[ids.CabalID]port.CallRecord{}, f.recordsErr
}

func TestCallRecords_recordsMergePendingSymbolsAndSkipEmptyInput(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	store := &fakeCallStore{
		records: map[ids.UserID]port.CallRecord{user: {Right: 1, Resolved: 2}},
		pending: map[ids.UserID][]string{user: {"AAPLx"}},
	}
	calls := app.NewCallRecords(store)
	period := domain.PeriodContaining(
		domain.PeriodAll,
		domain.PeriodContaining(domain.PeriodMonth, clock.Real{}.Now()).Start,
	)
	got, err := calls.Records(t.Context(), []ids.UserID{user}, nil, period)
	if err != nil || got[user].Right != 1 || len(got[user].PendingSymbols) != 1 {
		t.Fatalf("Records = %+v, %v, want the record with its pending symbol", got, err)
	}
	if empty, err := calls.Records(t.Context(), nil, nil, period); err != nil || len(empty) != 0 {
		t.Fatalf("Records(no users) = %v, %v, want empty", empty, err)
	}
	if empty, err := calls.CabalRecords(t.Context(), nil, period); err != nil || len(empty) != 0 || store.calls != 1 {
		t.Fatalf(
			"CabalRecords(no cabals) = %v, %v after %d store calls, want empty without a read",
			empty,
			err,
			store.calls,
		)
	}
}

func TestCallRecords_storeErrorsComeBack(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	user := []ids.UserID{ids.UserIDFrom(ids.Real{}.NewV7())}
	period := domain.PeriodContaining(domain.PeriodAll, clock.Real{}.Now())
	if _, err := app.NewCallRecords(&fakeCallStore{recordsErr: boom}).
		Records(t.Context(), user, nil, period); !errors.Is(
		err,
		boom,
	) {
		t.Errorf("Records records error = %v, want boom", err)
	}
	if _, err := app.NewCallRecords(&fakeCallStore{pendingErr: boom}).
		Records(t.Context(), user, nil, period); !errors.Is(
		err,
		boom,
	) {
		t.Errorf("Records pending error = %v, want boom", err)
	}
	cabals := []ids.CabalID{ids.CabalIDFrom(ids.Real{}.NewV7())}
	if _, err := app.NewCallRecords(&fakeCallStore{recordsErr: boom}).
		CabalRecords(t.Context(), cabals, period); !errors.Is(
		err,
		boom,
	) {
		t.Errorf("CabalRecords error = %v, want boom", err)
	}
}
