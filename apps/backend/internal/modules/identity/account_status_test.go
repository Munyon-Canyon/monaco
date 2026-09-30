package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

type accountPair struct {
	from domain.AccountStatus
	ev   domain.AccountEvent
}

func legalAccountMoves() map[accountPair]domain.AccountStatus {
	return map[accountPair]domain.AccountStatus{
		{domain.AccountActive, domain.AccountSuspend}:      domain.AccountSuspended,
		{domain.AccountSuspended, domain.AccountReinstate}: domain.AccountActive,
		{domain.AccountActive, domain.AccountBan}:          domain.AccountBanned,
		{domain.AccountSuspended, domain.AccountBan}:       domain.AccountBanned,
		{domain.AccountBanned, domain.AccountUnban}:        domain.AccountActive,
		{domain.AccountActive, domain.AccountDelete}:       domain.AccountDeleted,
		{domain.AccountSuspended, domain.AccountDelete}:    domain.AccountDeleted,
		{domain.AccountBanned, domain.AccountDelete}:       domain.AccountDeleted,
	}
}

func everyAccountPair() []accountPair {
	out := make([]accountPair, 0, 20)
	for _, from := range []domain.AccountStatus{
		domain.AccountActive, domain.AccountSuspended, domain.AccountBanned, domain.AccountDeleted,
	} {
		for _, ev := range []domain.AccountEvent{
			domain.AccountSuspend, domain.AccountReinstate, domain.AccountBan, domain.AccountUnban, domain.AccountDelete,
		} {
			out = append(out, accountPair{from, ev})
		}
	}
	return out
}

func TestNextAccountStatus_everyStatusAndEvent(t *testing.T) {
	t.Parallel()
	legal := legalAccountMoves()
	for _, p := range everyAccountPair() {
		want, ok := legal[p]
		t.Run(string(p.from)+"+"+string(p.ev), func(t *testing.T) {
			t.Parallel()
			got, err := domain.NextAccountStatus(p.from, p.ev)
			switch {
			case ok && err != nil:
				t.Fatalf("NextAccountStatus: %v", err)
			case !ok && errs.CodeOf(err) != errs.CodeAccountStatusTransition:
				t.Fatalf("err = %v, want %s", err, errs.CodeAccountStatusTransition)
			case !ok:
				want = p.from
			}
			if got != want {
				t.Errorf("NextAccountStatus = %s, want %s", got, want)
			}
		})
	}
}
