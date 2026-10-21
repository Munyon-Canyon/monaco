package funding_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	otherSolana = chain.SolanaAddress("FGETo8T8wMcN2wCjav8VK6eh3dLk63evNDPxzLSJra8B")
	phoneSID    = "session-phone"
	tabletSID   = "session-tablet"
)

func solanaRequest(micros uint64, to chain.SolanaAddress) domain.WithdrawalRequest {
	return domain.WithdrawalRequest{Amount: money.MicrosFromUint64(micros), To: to}
}

func evmRequest(t *testing.T, micros uint64) domain.WithdrawalRequest {
	t.Helper()
	r, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{
		Amount: money.MicrosFromUint64(micros).String(), Kind: "evm", Network: "base", To: evmTo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *withdrawFixture) authorize(
	t *testing.T, session string, issued time.Time, r domain.WithdrawalRequest,
) (app.WithdrawalAuthorization, error) {
	t.Helper()
	return app.NewAuthorizeWithdrawalHandler(f.uow, f.clock).Handle(f.ctx(t.Context()), app.AuthorizeWithdrawal{
		ID: ids.Real{}.NewV7(), UserID: f.user.ID, SessionID: session, IssuedAt: issued, Request: r,
	})
}

func (f *withdrawFixture) mustAuthorize(t *testing.T, r domain.WithdrawalRequest) uuid.UUID {
	t.Helper()
	granted, err := f.authorize(t, phoneSID, f.clock.Now(), r)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	return granted.ID
}

func (f *withdrawFixture) withdrawWith(
	ctx context.Context, session string, authorization uuid.UUID, r domain.WithdrawalRequest,
) (app.WithdrawResult, error) {
	return f.handler.Handle(ctx, app.Withdraw{
		ID: ids.Real{}.NewV7(), UserID: f.user.ID, Request: r, SessionID: session, Authorization: authorization,
	})
}

func (f *withdrawFixture) confirmAll(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE withdrawals SET status = 'confirmed', completed_at = submitted_at, tx_signature = tx_signature || id::text
			WHERE status = 'submitted'`,
	); err != nil {
		t.Fatal(err)
	}
}

func (f *withdrawFixture) consumed(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM withdrawal_authorizations WHERE consumed_at IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *withdrawFixture) expectRefused(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != want {
		t.Fatalf("Withdraw err = %v, want %s", err, want)
	}
	if rows := f.rows(t); len(rows) != 0 || len(f.transfers.builds) != 0 || f.consumed(t) != 0 {
		t.Fatalf("a refused withdrawal left rows = %+v, builds = %d, consumed = %d",
			rows, len(f.transfers.builds), f.consumed(t))
	}
}

func TestWithdrawStepUp_aNewDestinationNeedsAConfirmation(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, uuid.Nil, solanaRequest(withdrawMicros, withdrawTo))
	f.expectRefused(t, err, errs.CodeStepUpRequired)
}

func TestWithdrawStepUp_aConfirmationCoversExactlyTheAmountAndTheDestination(t *testing.T) {
	t.Parallel()
	for name, changed := range map[string]domain.WithdrawalRequest{
		"amount":  solanaRequest(withdrawMicros+1, withdrawTo),
		"address": solanaRequest(withdrawMicros, otherSolana),
		"network": evmRequest(t, withdrawMicros),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWithdrawFixture(t, 5_000_000, func(d *app.WithdrawDeps) { d.Actions = &privyActions{} })
			authorization := f.mustAuthorize(t, solanaRequest(withdrawMicros, withdrawTo))
			_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization, changed)
			f.expectRefused(t, err, errs.CodeStepUpMismatch)
			if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization,
				solanaRequest(withdrawMicros, withdrawTo)); err != nil {
				t.Fatalf("the exact request after a refused change = %v, want it to go through", err)
			}
		})
	}
}

func TestWithdrawStepUp_aConfirmationWorksOnce(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	authorization := f.mustAuthorize(t, request)
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization, request); err != nil {
		t.Fatalf("first withdrawal = %v", err)
	}
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization, request)
	if errs.CodeOf(err) != errs.CodeStepUpRequired || len(f.rows(t)) != 1 || f.consumed(t) != 1 {
		t.Fatalf("second withdrawal = %v rows = %+v consumed = %d, want step_up_required and one row",
			err, f.rows(t), f.consumed(t))
	}
}

func TestWithdrawStepUp_aConfirmationExpires(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	authorization := f.mustAuthorize(t, request)
	f.clock.Advance(domain.StepUpTTL)
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization, request)
	f.expectRefused(t, err, errs.CodeStepUpRequired)
}

func TestWithdrawStepUp_aConfirmationBelongsToTheSessionThatAskedForIt(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	authorization := f.mustAuthorize(t, request)
	_, err := f.withdrawWith(f.ctx(t.Context()), tabletSID, authorization, request)
	f.expectRefused(t, err, errs.CodeStepUpRequired)
}

func TestWithdrawStepUp_aConfirmationBelongsToTheUserThatAskedForIt(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	authorization := f.mustAuthorize(t, request)
	_, err := f.handler.Handle(f.ctx(t.Context()), app.Withdraw{
		ID: ids.Real{}.NewV7(), UserID: ids.UserIDFrom(ids.Real{}.NewV7()), Request: request, SessionID: phoneSID,
		Authorization: authorization,
	})
	if err == nil || f.consumed(t) != 0 || len(f.rows(t)) != 0 {
		t.Fatalf(
			"another user's withdrawal = %v consumed = %d rows = %+v, want it refused",
			err,
			f.consumed(t),
			f.rows(t),
		)
	}
}

func TestWithdrawStepUp_anUnknownConfirmationIsRequired(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	_, err := f.withdrawWith(
		f.ctx(t.Context()),
		phoneSID,
		ids.Real{}.NewV7(),
		solanaRequest(withdrawMicros, withdrawTo),
	)
	f.expectRefused(t, err, errs.CodeStepUpRequired)
}

func TestWithdrawStepUp_aKnownDestinationNeedsNoConfirmationBelowTheHighRiskAmount(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 3_000_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, f.mustAuthorize(t, request), request); err != nil {
		t.Fatalf("first withdrawal = %v", err)
	}
	if _, err := f.withdrawWith(
		f.ctx(t.Context()),
		phoneSID,
		uuid.Nil,
		request,
	); errs.CodeOf(
		err,
	) != errs.CodeStepUpRequired {
		t.Fatalf("a destination whose first withdrawal has not confirmed = %v, want step_up_required", err)
	}
	f.confirmAll(t)
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, uuid.Nil, request); err != nil {
		t.Fatalf("a confirmed destination = %v, want no confirmation needed", err)
	}
	f.confirmAll(t)
	high := solanaRequest(domain.HighRiskWithdrawalMicros, withdrawTo)
	if _, err := f.withdrawWith(
		f.ctx(t.Context()),
		phoneSID,
		uuid.Nil,
		high,
	); errs.CodeOf(
		err,
	) != errs.CodeStepUpRequired {
		t.Fatalf("a high-risk amount to a known destination = %v, want step_up_required", err)
	}
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, f.mustAuthorize(t, high), high); err != nil {
		t.Fatalf("a confirmed high-risk amount = %v", err)
	}
}

func TestWithdrawStepUp_aConfirmationIsCheckedEvenWhenTheWithdrawalNeedsNone(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, f.mustAuthorize(t, request), request); err != nil {
		t.Fatalf("first withdrawal = %v", err)
	}
	f.confirmAll(t)
	stale := f.mustAuthorize(t, solanaRequest(withdrawMicros+1, withdrawTo))
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, stale, request)
	if errs.CodeOf(err) != errs.CodeStepUpMismatch || len(f.rows(t)) != 1 {
		t.Fatalf("a mismatched confirmation on a known destination = %v rows = %+v", err, f.rows(t))
	}
}

func TestWithdrawStepUp_aEVMDestinationNeedsAConfirmationToo(t *testing.T) {
	t.Parallel()
	f := newEVMFixture(t, &privyActions{})
	request := evmRequest(t, withdrawMicros)
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, uuid.Nil, request)
	f.expectRefused(t, err, errs.CodeStepUpRequired)
	if _, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, f.mustAuthorize(t, request), request); err != nil {
		t.Fatalf("a confirmed EVM withdrawal = %v", err)
	}
}

func TestWithdrawStepUp_racingWithdrawalsSpendOneConfirmationOnce(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 10_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	authorization := f.mustAuthorize(t, request)
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { _, results[i] = f.withdrawWith(f.ctx(t.Context()), phoneSID, authorization, request) })
	}
	wg.Wait()
	refused := 0
	for _, err := range results {
		switch {
		case errs.CodeOf(err) == errs.CodeStepUpRequired:
			refused++
		case err != nil:
			t.Fatalf("Handle err = %v", err)
		}
	}
	if refused != 1 || len(f.rows(t)) != 1 || f.consumed(t) != 1 {
		t.Fatalf("errors = %v rows = %+v consumed = %d, want one withdrawal", results, f.rows(t), f.consumed(t))
	}
}

func TestAuthorizeWithdrawal_needsATokenIssuedRecently(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	request := solanaRequest(withdrawMicros, withdrawTo)
	now := f.clock.Now()
	for name, tc := range map[string]struct {
		session string
		issued  time.Time
		want    errs.Code
	}{
		"just issued":              {phoneSID, now, ""},
		"at the freshness limit":   {phoneSID, now.Add(-domain.StepUpFreshness), ""},
		"a copied older token":     {phoneSID, now.Add(-domain.StepUpFreshness - time.Second), errs.CodeReauthRequired},
		"an hour old":              {phoneSID, now.Add(-time.Hour), errs.CodeReauthRequired},
		"a dev token has no clock": {"", now.Add(-time.Hour), ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := f.authorize(t, tc.session, tc.issued, request)
			if tc.want == "" && err != nil || tc.want != "" && errs.CodeOf(err) != tc.want {
				t.Fatalf("authorize = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWithdrawStepUp_aDatabaseFailureReadingTheDestinationHistoryWritesNothing(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE withdrawals RENAME TO withdrawals_moved`); err != nil {
		t.Fatal(err)
	}
	_, err := f.withdrawWith(f.ctx(t.Context()), phoneSID, uuid.Nil, solanaRequest(withdrawMicros, withdrawTo))
	if err == nil || errs.CodeOf(err) == errs.CodeStepUpRequired || len(f.transfers.builds) != 0 {
		t.Fatalf("Withdraw err = %v builds = %d, want the database error", err, len(f.transfers.builds))
	}
}
