package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeUserCreated          Type = "user.created"
	TypeUserAuthStateChanged Type = "user.auth_state_changed"
	TypeUserProfileUpdated   Type = "user.profile_updated"
)

const userAggregate = "user"

type UserCreated struct {
	V             int       `json:"v"`
	UserID        uuid.UUID `json:"user_id"        pii:"true"`
	LoginProvider string    `json:"login_provider"`
	CreatedAt     time.Time `json:"created_at"`
}

func (UserCreated) Type() Type { return TypeUserCreated }

func (UserCreated) AggregateType() string { return userAggregate }

func (e UserCreated) AggregateID() uuid.UUID { return e.UserID }

type UserAuthStateChanged struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Cause  string    `json:"cause"`
	At     time.Time `json:"at"`
}

func (UserAuthStateChanged) Type() Type { return TypeUserAuthStateChanged }

func (UserAuthStateChanged) AggregateType() string { return userAggregate }

func (e UserAuthStateChanged) AggregateID() uuid.UUID { return e.UserID }

type UserProfileUpdated struct {
	V           int       `json:"v"`
	UserID      uuid.UUID `json:"user_id"      pii:"true"`
	Fields      []string  `json:"fields"`
	Handle      string    `json:"handle"       pii:"true"`
	DisplayName string    `json:"display_name" pii:"true"`
	PhotoURL    string    `json:"photo_url"    pii:"true"`
}

func (UserProfileUpdated) Type() Type { return TypeUserProfileUpdated }

func (UserProfileUpdated) AggregateType() string { return userAggregate }

func (e UserProfileUpdated) AggregateID() uuid.UUID { return e.UserID }
