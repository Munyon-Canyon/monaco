package cabal_test

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func accessStatuses() []domain.AccessStatus {
	return []domain.AccessStatus{
		domain.AccessPending, domain.AccessApproved, domain.AccessDenied, domain.AccessRevoked, domain.AccessExpired,
	}
}

func accessEvents() []domain.AccessEvent {
	return []domain.AccessEvent{domain.AccessApprove, domain.AccessDeny, domain.AccessRevoke, domain.AccessExpire}
}

func legalAccessMoves() map[domain.AccessEvent]domain.AccessStatus {
	return map[domain.AccessEvent]domain.AccessStatus{
		domain.AccessApprove: domain.AccessApproved,
		domain.AccessDeny:    domain.AccessDenied,
		domain.AccessRevoke:  domain.AccessRevoked,
		domain.AccessExpire:  domain.AccessExpired,
	}
}

func wantRefusal(from domain.AccessStatus) errs.Code {
	switch from {
	case domain.AccessPending:
		return ""
	case domain.AccessExpired:
		return errs.CodeInviteExpired
	case domain.AccessApproved, domain.AccessDenied, domain.AccessRevoked:
		return errs.CodeAccessRequestNotPending
	}
	return errs.CodeInternal
}

func TestNext_onlyAPendingRowMovesAndEveryOtherStatusIsTerminal(t *testing.T) {
	t.Parallel()
	legal := legalAccessMoves()
	for _, from := range accessStatuses() {
		for _, ev := range accessEvents() {
			got, err := domain.Next(from, ev)
			if refusal := wantRefusal(from); refusal != "" {
				wantCode(t, string(from)+"+"+string(ev), err, refusal)
				if got != from {
					t.Errorf("Next(%s, %s) moved to %s on an error, want it to stay", from, ev, got)
				}
			} else if err != nil || got != legal[ev] {
				t.Errorf("Next(%s, %s) = %s, %v; want %s", from, ev, got, err, legal[ev])
			}
		}
	}
}

func TestNext_unknownStatusesAndEventsFailInternalAndStay(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		from domain.AccessStatus
		ev   domain.AccessEvent
	}{
		"unknown status":     {"limbo", domain.AccessApprove},
		"unknown event":      {domain.AccessPending, "poke"},
		"both unknown":       {"limbo", "poke"},
		"empty status":       {"", domain.AccessDeny},
		"terminal and empty": {domain.AccessApproved, ""},
	} {
		got, err := domain.Next(tt.from, tt.ev)
		wantCode(t, name, err, errs.CodeInternal)
		if got != tt.from {
			t.Errorf("%s: status = %s, want it unchanged at %s", name, got, tt.from)
		}
	}
}

func TestNext_aRowDecidesExactlyOnceWhateverTheEventOrder(t *testing.T) {
	t.Parallel()
	legal := legalAccessMoves()
	rapid.Check(t, func(t *rapid.T) {
		events := rapid.SliceOfN(rapid.SampledFrom(accessEvents()), 1, 6).Draw(t, "events")
		status := domain.AccessPending
		for i, ev := range events {
			next, err := domain.Next(status, ev)
			if i == 0 && (err != nil || next != legal[ev]) {
				t.Fatalf("first event %s: %s, %v; want %s", ev, next, err, legal[ev])
			}
			if i > 0 && (err == nil || next != status) {
				t.Fatalf(
					"event %d (%s) after %s: %s, %v; want a refusal that keeps %s",
					i,
					ev,
					status,
					next,
					err,
					status,
				)
			}
			status = next
		}
	})
}

func TestInviteExpiry_isSevenDaysAfterTheInviteWasMade(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0).UTC()
	if got := domain.InviteExpiry(now); got.Sub(now) != 7*24*time.Hour || domain.InviteLifetime != 7*24*time.Hour {
		t.Fatalf(
			"InviteExpiry(now) = now + %s and InviteLifetime = %s, want 7 days both",
			got.Sub(now),
			domain.InviteLifetime,
		)
	}
}

func TestAccessRequestCheckNotExpired_anInviteLivesThroughItsExpiryInstantAndARequestNeverExpires(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0).UTC()
	invite := func(expires time.Time) domain.AccessRequest {
		return domain.AccessRequest{Direction: domain.DirectionInvite, ExpiresAt: expires}
	}
	for _, tt := range []struct {
		name string
		req  domain.AccessRequest
		want errs.Code
	}{
		{"expires a second from now", invite(now.Add(time.Second)), ""},
		{"expires right now", invite(now), ""},
		{"expired a nanosecond ago", invite(now.Add(-time.Nanosecond)), errs.CodeInviteExpired},
		{"expired a lifetime ago", invite(now.Add(-domain.InviteLifetime)), errs.CodeInviteExpired},
		{"an invite with no expiry fails closed", domain.AccessRequest{Direction: domain.DirectionInvite}, errs.CodeInviteExpired},
		{"a request with no expiry", domain.AccessRequest{Direction: domain.DirectionRequest}, ""},
		{"a request older than any invite", domain.AccessRequest{
			Direction: domain.DirectionRequest, ExpiresAt: now.Add(-domain.InviteLifetime),
		}, ""},
	} {
		err := tt.req.CheckNotExpired(now)
		if tt.want == "" {
			if err != nil {
				t.Errorf("%s: %v, want it live", tt.name, err)
			}
			continue
		}
		wantCode(t, tt.name, err, tt.want)
	}
}

func TestAccessRequestCheckNotExpired_anInviteIsLiveExactlyUntilItsExpiry(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0).UTC()
	rapid.Check(t, func(t *rapid.T) {
		offset := time.Duration(
			rapid.Int64Range(-int64(2*domain.InviteLifetime), int64(2*domain.InviteLifetime)).Draw(t, "offset"),
		)
		req := domain.AccessRequest{Direction: domain.DirectionInvite, ExpiresAt: now.Add(offset)}
		err := req.CheckNotExpired(now)
		if live := offset >= 0; live != (err == nil) || (err != nil && errs.CodeOf(err) != errs.CodeInviteExpired) {
			t.Fatalf("invite expiring at now%+d: err = %v, want live = %t", offset, err, live)
		}
	})
}
