package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type callBoardFake struct {
	run        *sqlc.CallBoardRun
	trigger    *time.Time
	runErr     error
	triggerErr error
	boardErr   error
	runs       int
}

func (f *callBoardFake) CallBoardLastRun(context.Context) (sqlc.CallBoardRun, error) {
	if f.runErr != nil || f.run == nil {
		return sqlc.CallBoardRun{}, orNoRows(f.runErr)
	}
	return *f.run, nil
}

func (f *callBoardFake) OldestCallBoardTrigger(context.Context) (time.Time, error) {
	if f.triggerErr != nil || f.trigger == nil {
		return time.Time{}, orNoRows(f.triggerErr)
	}
	return *f.trigger, nil
}

func (f *callBoardFake) Run(context.Context, time.Time) (int, error) {
	f.runs++
	return 12, f.boardErr
}

func (f *callBoardFake) poller(clock *testkit.Clock) adapters.CallBoardPoller {
	return adapters.CallBoardPoller{Reads: f, Runner: f, Clock: clock}
}

func TestCallBoardPoller_runsOnlyWhenTheScheduleSaysSo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	last := func(d time.Duration) *sqlc.CallBoardRun { return &sqlc.CallBoardRun{FinishedAt: now.Add(-d)} }
	for name, tc := range map[string]struct {
		fake *callBoardFake
		runs int
	}{
		"the very first tick":           {&callBoardFake{}, 1},
		"a recent run and no trigger":   {&callBoardFake{run: last(time.Second)}, 0},
		"a run four minutes ago":        {&callBoardFake{run: last(4 * time.Minute)}, 0},
		"a run five minutes ago":        {&callBoardFake{run: last(5 * time.Minute)}, 1},
		"a trigger under a second old":  {&callBoardFake{run: last(time.Minute), trigger: ago(900 * time.Millisecond)}, 0},
		"a trigger a second old":        {&callBoardFake{run: last(time.Minute), trigger: ago(time.Second)}, 1},
		"a trigger and no earlier runs": {&callBoardFake{trigger: ago(time.Second)}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			report, err := tc.fake.poller(testkit.NewClock(now)).Tick(t.Context())
			if err != nil || tc.fake.runs != tc.runs {
				t.Fatalf("Tick() = %+v, %v with %d runs, want %d", report, err, tc.fake.runs, tc.runs)
			}
			if tc.runs == 1 && report.Changed != 12 {
				t.Fatalf("report = %+v, want 12 rows changed", report)
			}
		})
	}
}

func TestCallBoardPoller_returnsEveryFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	for name, fake := range map[string]*callBoardFake{
		"last run read": {runErr: boom},
		"trigger read":  {triggerErr: boom},
		"board run":     {boardErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := fake.poller(testkit.NewClock(now)).
				Tick(t.Context()); errs.CodeOf(
				err,
			) != errs.CodeUpstreamUnavailable {
				t.Fatalf("Tick() = %v, want the upstream error", err)
			}
		})
	}
}
