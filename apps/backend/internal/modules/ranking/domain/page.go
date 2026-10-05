package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Subject struct {
	ID         uuid.UUID
	Name       string
	Handle     *string
	PictureURL *string
}

type Entry struct {
	Rank       int
	Subject    Subject
	Value      money.Micros
	PnL        money.SignedMicros
	Return     *Bps
	Flags      []Flag
	ComputedAt time.Time
	PricesAsOf time.Time
}

type BoardPage struct {
	RunID      *uuid.UUID
	ComputedAt time.Time
	PricesAsOf time.Time
	Rows       []Entry
	Me         *Entry
	NextCursor *string
}

type Run struct {
	ID         uuid.UUID
	Rev        int32
	PricesAsOf time.Time
	FinishedAt time.Time
}
