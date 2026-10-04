package events

import "github.com/google/uuid"

const (
	TypeAdminGranted Type = "admin.granted"
	TypeAdminRevoked Type = "admin.revoked"
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
