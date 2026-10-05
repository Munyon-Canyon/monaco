package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestNextPayoutStep_signsOnlyWhenNoAttemptCanStillLand(t *testing.T) {
	t.Parallel()
	attempt := func(n int16, s domain.PayoutStatus) domain.PayoutAttempt {
		return domain.PayoutAttempt{Number: n, Status: s}
	}
	for _, c := range []struct {
		job     domain.CashOutStatus
		selling bool
		latest  domain.PayoutAttempt
		want    domain.PayoutStep
	}{
		{domain.CashOutStarted, false, domain.PayoutAttempt{}, domain.PayoutSign},
		{domain.CashOutStarted, true, domain.PayoutAttempt{}, domain.PayoutAwaitSale},
		{domain.CashOutSelling, true, domain.PayoutAttempt{}, domain.PayoutAwaitSale},
		{domain.CashOutPaying, true, domain.PayoutAttempt{}, domain.PayoutSign},
		{domain.CashOutPaying, false, attempt(1, domain.PayoutSigned), domain.PayoutSend},
		{domain.CashOutPaying, false, attempt(1, domain.PayoutBroadcast), domain.PayoutCheck},
		{domain.CashOutPaying, false, attempt(1, domain.PayoutConfirmed), domain.PayoutCheck},
		{domain.CashOutPaying, false, attempt(2, domain.PayoutExpired), domain.PayoutSign},
		{domain.CashOutPaying, false, attempt(3, domain.PayoutExpired), domain.PayoutGiveUp},
		{domain.CashOutPaying, false, attempt(3, domain.PayoutFailed), domain.PayoutGiveUp},
		{domain.CashOutCompleted, false, attempt(1, domain.PayoutConfirmed), domain.PayoutIdle},
		{domain.CashOutPartial, true, attempt(1, domain.PayoutConfirmed), domain.PayoutIdle},
		{domain.CashOutFailed, false, attempt(3, domain.PayoutExpired), domain.PayoutIdle},
	} {
		if got := domain.NextPayoutStep(c.job, c.selling, c.latest); got != c.want {
			t.Errorf("NextPayoutStep(%s, %t, %+v) = %d, want %d", c.job, c.selling, c.latest, got, c.want)
		}
	}
}

func TestPayoutReading_verdicts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   domain.PayoutReading
		want domain.PayoutVerdict
	}{
		{domain.PayoutReading{State: domain.PayoutFinalized}, domain.PayoutLanded},
		{domain.PayoutReading{State: domain.PayoutFinalized, Failed: true}, domain.PayoutRejected},
		{domain.PayoutReading{State: domain.PayoutNotFound, Expired: true}, domain.PayoutLapsed},
		{domain.PayoutReading{State: domain.PayoutNotFound}, domain.PayoutMissing},
		{domain.PayoutReading{State: domain.PayoutProcessing, Failed: true}, domain.PayoutInFlight},
	} {
		if got := c.in.Verdict(); got != c.want {
			t.Errorf("Verdict(%+v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParsePayoutStatus(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "signed", "broadcast", "confirmed", "expired", "failed"} {
		if got, err := domain.ParsePayoutStatus(raw); err != nil || string(got) != raw {
			t.Fatalf("ParsePayoutStatus(%q) = %q, %v", raw, got, err)
		}
	}
	if _, err := domain.ParsePayoutStatus("dropped"); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ParsePayoutStatus(dropped) = %v, want decode_failed", err)
	}
}
