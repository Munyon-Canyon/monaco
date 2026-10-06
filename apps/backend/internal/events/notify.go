package events

import "github.com/google/uuid"

const (
	TypeNotifyTestRequested Type = "notify.test_requested"
	TypeNotificationSent    Type = "notification.sent"
)

type NotifyTestRequested struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
}

func (NotifyTestRequested) Type() Type { return TypeNotifyTestRequested }

func (NotifyTestRequested) AggregateType() string { return userAggregate }

func (e NotifyTestRequested) AggregateID() uuid.UUID { return e.UserID }

type NotificationSent struct {
	V              int       `json:"v"`
	NotificationID uuid.UUID `json:"notification_id"`
	UserID         uuid.UUID `json:"user_id"         pii:"true"`
	Kind           string    `json:"kind"`
	SourceEventID  uuid.UUID `json:"source_event_id"`
}

func (NotificationSent) Type() Type { return TypeNotificationSent }

func (NotificationSent) AggregateType() string { return "notification" }

func (e NotificationSent) AggregateID() uuid.UUID { return e.NotificationID }
