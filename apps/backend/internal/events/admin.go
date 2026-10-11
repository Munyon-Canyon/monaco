package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeAdminGranted Type = "admin.granted"
	TypeAdminRevoked Type = "admin.revoked"

	TypeAdminApprovalRequested Type = "admin.approval_requested"
	TypeAdminCabalBanApproved  Type = "admin.cabal_ban_approved"
)

type AdminGranted struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	Role   string    `json:"role"`
}

func (AdminGranted) Type() Type { return TypeAdminGranted }

func (AdminGranted) AggregateType() string { return "admin" }

func (e AdminGranted) AggregateID() uuid.UUID { return e.UserID }

type AdminRevoked struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
}

func (AdminRevoked) Type() Type { return TypeAdminRevoked }

func (AdminRevoked) AggregateType() string { return "admin" }

func (e AdminRevoked) AggregateID() uuid.UUID { return e.UserID }

type AdminApprovalRequested struct {
	V           int       `json:"v"`
	ApprovalID  uuid.UUID `json:"approval_id"`
	Action      string    `json:"action"`
	TargetID    uuid.UUID `json:"target_id"`
	RequestedBy uuid.UUID `json:"requested_by" pii:"true"`
	Reason      string    `json:"reason"       pii:"true"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (AdminApprovalRequested) Type() Type { return TypeAdminApprovalRequested }

func (AdminApprovalRequested) AggregateType() string { return "admin_approval" }

func (e AdminApprovalRequested) AggregateID() uuid.UUID { return e.ApprovalID }

type AdminCabalBanApproved struct {
	V           int       `json:"v"`
	ApprovalID  uuid.UUID `json:"approval_id"`
	CabalID     uuid.UUID `json:"cabal_id"`
	RequestedBy uuid.UUID `json:"requested_by" pii:"true"`
	ApprovedBy  uuid.UUID `json:"approved_by"  pii:"true"`
	Reason      string    `json:"reason"       pii:"true"`
}

func (AdminCabalBanApproved) Type() Type { return TypeAdminCabalBanApproved }

func (AdminCabalBanApproved) AggregateType() string { return "admin_approval" }

func (e AdminCabalBanApproved) AggregateID() uuid.UUID { return e.ApprovalID }
