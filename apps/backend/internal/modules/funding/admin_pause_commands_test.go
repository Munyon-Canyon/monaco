package funding_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func commandAction(t *testing.T, kind events.AdminActionKind, cabal ids.CabalID) *events.AdminAction {
	t.Helper()
	return commandActionSeeded(t, 60, kind, cabal)
}

func commandActionSeeded(
	t *testing.T, seed uint64, kind events.AdminActionKind, cabal ids.CabalID,
) *events.AdminAction {
	t.Helper()
	reason, err := events.NewReason("investigating")
	if err != nil {
		t.Fatal(err)
	}
	action, err := events.NewAdminAction(testkit.NewIDs(seed).NewV7(), ids.UserIDFrom(testkit.NewIDs(seed+1).NewV7()),
		kind, events.AdminTargetCabal, cabal.String(), reason, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &action
}

func TestAdminPauseCommand_AuditsEvenWhenAnotherReasonHoldsThePause(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	mustPause(t, env, cabal.ID, domain.PauseReasonExternalDeposit)
	_, err := env.pause.Handle(opsContext(t), app.PauseCabal{
		CabalID:     &cabal.ID,
		Reason:      domain.PauseReasonOps,
		AdminAction: commandAction(t, events.AdminActionOpsPause, cabal.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	action := adminActionPayload(t, env.pool, events.AdminActionOpsPause)
	if n := countEvents(t, env.pool, events.TypeCabalPaused); n != 1 ||
		!slices.Equal(pausedReasons(t, action.Before), []string{"external_deposit"}) ||
		!slices.Equal(pausedReasons(t, action.After), []string{"external_deposit", "ops"}) {
		t.Fatalf("cabal.paused=%d before=%s after=%s, want the one event and both reasons audited",
			n, action.Before, action.After)
	}
}

func TestAdminPauseCommand_AuditsAReasonOnceWhenTwoPausesHoldIt(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	mustPause(t, env, cabal.ID, domain.PauseReasonExternalDeposit)
	mustPause(t, env, cabal.ID, domain.PauseReasonExternalDeposit)
	_, err := env.pause.Handle(opsContext(t), app.PauseCabal{
		CabalID:     &cabal.ID,
		Reason:      domain.PauseReasonOps,
		AdminAction: commandAction(t, events.AdminActionOpsPause, cabal.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	action := adminActionPayload(t, env.pool, events.AdminActionOpsPause)
	if !slices.Equal(pausedReasons(t, action.Before), []string{"external_deposit"}) ||
		!slices.Equal(pausedReasons(t, action.After), []string{"external_deposit", "ops"}) {
		t.Fatalf("before=%s after=%s, want each reason listed once in order", action.Before, action.After)
	}
}

func TestAdminPause_ConcurrentOnlyOneWins(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	const racers = 2
	start := make(chan struct{})
	results := make([]error, racers)
	var group errgroup.Group
	for i := range racers {
		action := commandActionSeeded(t, uint64(100+10*i), events.AdminActionOpsPause, cabal.ID)
		group.Go(func() error {
			<-start
			_, results[i] = env.pause.Handle(opsContext(t), app.PauseCabal{
				CabalID: &cabal.ID, Reason: domain.PauseReasonOps, AdminAction: action,
			})
			return nil
		})
	}
	close(start)
	_ = group.Wait()
	var won, refused int
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case codeOrEmpty(err) == errs.CodeAlreadyPaused:
			refused++
		default:
			t.Fatalf("pause error = %v, want nil or already_paused", err)
		}
	}
	open := openPauses(t, env.pool)
	audits := countEvents(t, env.pool, events.TypeAdminAction)
	if won != 1 || refused != 1 || !slices.Equal(open, []string{"ops"}) || audits != 1 {
		t.Fatalf("won=%d refused=%d open=%v admin.action=%d, want one winner, one ops row and one audit",
			won, refused, open, audits)
	}
}

func TestAdminCommands_RefuseWithoutChangingAnything(t *testing.T) {
	t.Parallel()
	ops := domain.PauseReasonOps
	cases := map[string]struct {
		run  func(t *testing.T, env pauseEnv, cabal ids.CabalID) error
		want errs.Code
		open []string
	}{
		"a second ops pause": {
			run: func(t *testing.T, env pauseEnv, cabal ids.CabalID) error {
				t.Helper()
				mustPause(t, env, cabal, ops)
				_, err := env.pause.Handle(opsContext(t), app.PauseCabal{
					CabalID: &cabal, Reason: ops, AdminAction: commandAction(t, events.AdminActionOpsPause, cabal),
				})
				return err
			},
			want: errs.CodeAlreadyPaused, open: []string{"ops"},
		},
		"a resume with nothing open": {
			run: func(t *testing.T, env pauseEnv, cabal ids.CabalID) error {
				t.Helper()
				return env.resume.Handle(opsContext(t), app.ResumeCabal{
					CabalID: &cabal, AdminAction: commandAction(t, events.AdminActionOpsResume, cabal),
				})
			},
			want: errs.CodeNoOpsPause,
		},
		"a resume with only an external-deposit pause": {
			run: func(t *testing.T, env pauseEnv, cabal ids.CabalID) error {
				t.Helper()
				mustPause(t, env, cabal, domain.PauseReasonExternalDeposit)
				return env.resume.Handle(opsContext(t), app.ResumeCabal{
					CabalID: &cabal, AdminAction: commandAction(t, events.AdminActionOpsResume, cabal),
				})
			},
			want: errs.CodeNoOpsPause, open: []string{"external_deposit"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newPauseEnv(t)
			cabal := testkit.NewCabal(t, env.pool)
			if err := tc.run(t, env, cabal.ID); errs.CodeOf(err) != tc.want {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
			if got := openPauses(t, env.pool); !slices.Equal(got, tc.open) {
				t.Fatalf("open pauses = %v, want %v", got, tc.open)
			}
			if n := countEvents(t, env.pool, events.TypeAdminAction); n != 0 {
				t.Fatalf("admin.action events = %d, want none from a refused command", n)
			}
		})
	}
}

func TestAdminCommands_RollBackWhenTheAuditCannotBeAppended(t *testing.T) {
	t.Parallel()
	ops := domain.PauseReasonOps
	cases := map[string]func(t *testing.T, env pauseEnv, cabal ids.CabalID) error{
		"pause": func(t *testing.T, env pauseEnv, cabal ids.CabalID) error {
			t.Helper()
			mustPause(t, env, cabal, domain.PauseReasonExternalDeposit)
			_, err := env.pause.Handle(t.Context(), app.PauseCabal{
				CabalID: &cabal, Reason: ops, AdminAction: commandAction(t, events.AdminActionOpsPause, cabal),
			})
			return err
		},
		"resume": func(t *testing.T, env pauseEnv, cabal ids.CabalID) error {
			t.Helper()
			mustPause(t, env, cabal, domain.PauseReasonExternalDeposit)
			mustPause(t, env, cabal, ops)
			return env.resume.Handle(t.Context(), app.ResumeCabal{
				CabalID: &cabal, AdminAction: commandAction(t, events.AdminActionOpsResume, cabal),
			})
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newPauseEnv(t)
			cabal := testkit.NewCabal(t, env.pool)
			if err := run(t, env, cabal.ID); err == nil {
				t.Fatal("error = nil without an actor to attribute the audit to")
			}
			open := openPauses(t, env.pool)
			if name == "pause" && !slices.Equal(open, []string{"external_deposit"}) ||
				name == "resume" && !slices.Equal(open, []string{"external_deposit", "ops"}) {
				t.Fatalf("open pauses = %v, want the failed command rolled back", open)
			}
		})
	}
}

func adminContext(t *testing.T, id string) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: id, Role: "operator"})
}

func TestAdminPause_RefusesAnActorThatIsNotAnAdmin(t *testing.T) {
	t.Parallel()
	admin := testkit.NewIDs(70).NewV7().String()
	for name, ctx := range map[string]context.Context{
		"no actor":                   t.Context(),
		"a user":                     auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: admin}),
		"an admin without a user id": adminContext(t, "nope"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := adapters.HTTP{IDs: testkit.NewIDs(71)}
			body := &api.PauseAdminAllJSONRequestBody{Reason: "investigating"}
			_, err := h.PauseAdminAll(ctx, api.PauseAdminAllRequestObject{Body: body})
			if errs.CodeOf(err) != errs.CodeAdminForbidden {
				t.Fatalf("PauseAdminAll err = %v, want %s", err, errs.CodeAdminForbidden)
			}
		})
	}
}

func TestAdminPause_ReportsAFailedReadOfTheNewState(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	closed, err := pgxpool.NewWithConfig(t.Context(), env.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	h := adapters.HTTP{Pause: env.pause, Resume: env.resume, Reads: closed, IDs: testkit.NewIDs(72)}
	ctx := adminContext(t, testkit.NewIDs(73).NewV7().String())
	body := &api.PauseAdminAllJSONRequestBody{Reason: "investigating"}
	if _, err := h.PauseAdminAll(
		ctx,
		api.PauseAdminAllRequestObject{Body: body},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("PauseAdminAll err = %v, want %s from the state read", err, errs.CodeInternal)
	}
	if _, err := h.ResumeAdminAll(
		ctx,
		api.ResumeAdminAllRequestObject{Body: body},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("ResumeAdminAll err = %v, want %s from the state read", err, errs.CodeInternal)
	}
	if got := openPauses(t, env.pool); len(got) != 0 {
		t.Fatalf("open pauses = %v, want the resume to have cleared the pause", got)
	}
}
