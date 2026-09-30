package domain

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const InviteLifetime = 7 * 24 * time.Hour

type Direction string

const (
	DirectionRequest Direction = "request"
	DirectionInvite  Direction = "invite"
)

type AccessStatus string

const (
	AccessPending  AccessStatus = "pending"
	AccessApproved AccessStatus = "approved"
	AccessDenied   AccessStatus = "denied"
	AccessRevoked  AccessStatus = "revoked"
	AccessExpired  AccessStatus = "expired"
)

type AccessEvent string

const (
	AccessApprove AccessEvent = "approve"
	AccessDeny    AccessEvent = "deny"
	AccessRevoke  AccessEvent = "revoke"
	AccessExpire  AccessEvent = "expire"
)

func (s AccessStatus) known() bool {
	switch s {
	case AccessPending, AccessApproved, AccessDenied, AccessRevoked, AccessExpired:
		return true
	}
	return false
}

func (e AccessEvent) known() bool {
	switch e {
	case AccessApprove, AccessDeny, AccessRevoke, AccessExpire:
		return true
	}
	return false
}

type accessStep struct {
	from AccessStatus
	ev   AccessEvent
}

func transitions() map[accessStep]AccessStatus {
	return map[accessStep]AccessStatus{
		{AccessPending, AccessApprove}: AccessApproved,
		{AccessPending, AccessDeny}:    AccessDenied,
		{AccessPending, AccessRevoke}:  AccessRevoked,
		{AccessPending, AccessExpire}:  AccessExpired,
	}
}

func Next(from AccessStatus, ev AccessEvent) (AccessStatus, error) {
	const op = "cabal.Next"
	attrs := []slog.Attr{slog.String("from", string(from)), slog.String("event", string(ev))}
	if !from.known() || !ev.known() {
		return from, errs.New(errs.CodeInternal, op, attrs...)
	}
	if next, ok := transitions()[accessStep{from, ev}]; ok {
		return next, nil
	}
	if from == AccessExpired {
		return from, errs.New(errs.CodeInviteExpired, op, attrs...)
	}
	return from, errs.New(errs.CodeAccessRequestNotPending, op, attrs...)
}

func InviteExpiry(now time.Time) time.Time { return now.Add(InviteLifetime) }

func (r AccessRequest) CheckNotExpired(now time.Time) error {
	if r.Direction == DirectionInvite && r.ExpiresAt.Before(now) {
		return errs.New(errs.CodeInviteExpired, "cabal.AccessRequest.CheckNotExpired",
			slog.Time("expires_at", r.ExpiresAt))
	}
	return nil
}
