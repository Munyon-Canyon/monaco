package ids

import (
	"database/sql/driver"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type ID[T any] struct{ u uuid.UUID }

type (
	user     struct{}
	cabal    struct{}
	event    struct{}
	swap     struct{}
	proposal struct{}
)

type (
	UserID     = ID[user]
	CabalID    = ID[cabal]
	EventID    = ID[event]
	SwapID     = ID[swap]
	ProposalID = ID[proposal]
)

type Generator interface {
	NewV7() uuid.UUID
}

type Real struct{}

func (Real) NewV7() uuid.UUID {
	u, _ := uuid.NewV7()
	return u
}

func New[T any](g Generator) ID[T] { return ID[T]{u: g.NewV7()} }

func Parse[T any](raw string) (ID[T], error) {
	u, err := uuid.Parse(raw)
	if err != nil || u.String() != raw || u.Version() != 7 || u.Variant() != uuid.RFC4122 {
		return ID[T]{}, errs.New(errs.CodeInvalidInput, "ids.Parse", slog.String("raw", raw))
	}
	return ID[T]{u: u}, nil
}

func ParseUserID(raw string) (UserID, error) { return Parse[user](raw) }

func ParseCabalID(raw string) (CabalID, error) { return Parse[cabal](raw) }

func ParseEventID(raw string) (EventID, error) { return Parse[event](raw) }

func EventIDFrom(u uuid.UUID) EventID { return EventID{u: u} }

func SwapIDFrom(u uuid.UUID) SwapID { return SwapID{u: u} }

func CabalIDFrom(u uuid.UUID) CabalID { return CabalID{u: u} }

func ProposalIDFrom(u uuid.UUID) ProposalID { return ProposalID{u: u} }

func (id ID[T]) IsZero() bool { return id.u == uuid.Nil }

func (id ID[T]) String() string { return id.u.String() }

func (id ID[T]) UUID() uuid.UUID { return id.u }

func (id ID[T]) MarshalText() ([]byte, error) {
	if id.IsZero() {
		return nil, errs.New(errs.CodeInternal, "ids.ID.MarshalText")
	}
	return []byte(id.String()), nil
}

func (id *ID[T]) UnmarshalText(text []byte) error {
	v, err := Parse[T](string(text))
	if err != nil {
		return err
	}
	*id = v
	return nil
}

func (id ID[T]) Value() (driver.Value, error) {
	if id.IsZero() {
		return nil, errs.New(errs.CodeInternal, "ids.ID.Value")
	}
	return id.String(), nil
}

func (id *ID[T]) Scan(src any) error {
	var raw string
	switch s := src.(type) {
	case string:
		raw = s
	case []byte:
		raw = string(s)
	default:
		return errs.New(errs.CodeDecodeFailed, "ids.ID.Scan", slog.String("type", fmt.Sprintf("%T", src)))
	}
	v, err := Parse[T](raw)
	if err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, "ids.ID.Scan")
	}
	*id = v
	return nil
}
