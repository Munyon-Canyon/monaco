package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func registrations() []Registration {
	return []Registration{
		Register[SystemPinged](TypeSystemPinged, 1),
	}
}

type Registration struct {
	typ     Type
	current int
	fields  []Field
	decode  func(payload []byte, attrs []slog.Attr) (Event, error)
}

type Field struct {
	Name   string
	GoType string
}

type Entry struct {
	Type    Type
	Subject string
	Version int
	Fields  []Field
}

func Register[E Event](t Type, current int) Registration {
	var zero E
	if zero.Type() != t {
		panic(fmt.Sprintf("events: %s registered as %s", zero.Type(), t))
	}
	if current < 1 {
		panic(fmt.Sprintf("events: %s registered at version %d", t, current))
	}
	return Registration{typ: t, current: current, fields: fieldsOf(reflect.TypeFor[E]()), decode: decodeInto[E]}
}

func decodeInto[E Event](payload []byte, attrs []slog.Attr) (Event, error) {
	var e E
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, "events.Decode", attrs...)
	}
	return e, nil
}

func fieldsOf(t reflect.Type) []Field {
	fields := make([]Field, 0, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		fields = append(fields, Field{Name: name, GoType: f.Type.String()})
	}
	return fields
}

type registry map[Type]Registration

func newRegistry(regs []Registration) registry {
	r := make(registry, len(regs))
	for _, reg := range regs {
		if _, dup := r[reg.typ]; dup {
			panic(fmt.Sprintf("events: %s registered twice", reg.typ))
		}
		r[reg.typ] = reg
	}
	return r
}

func Subjects() []string {
	r := newRegistry(registrations())
	subjects := make([]string, 0, len(r))
	for _, t := range slices.Sorted(maps.Keys(r)) {
		subjects = append(subjects, t.Subject())
	}
	return subjects
}

func Catalog() []Entry {
	r := newRegistry(registrations())
	entries := make([]Entry, 0, len(r))
	for _, t := range slices.Sorted(maps.Keys(r)) {
		reg := r[t]
		entries = append(entries, Entry{Type: t, Subject: t.Subject(), Version: reg.current, Fields: reg.fields})
	}
	return entries
}

func Decode(t Type, v int, payload []byte) (Event, error) {
	return newRegistry(registrations()).decode(t, v, payload)
}

func (r registry) decode(t Type, v int, payload []byte) (Event, error) {
	const op = "events.Decode"
	attrs := []slog.Attr{slog.String("type", string(t)), slog.Int("v", v)}
	reg, ok := r[t]
	if !ok || v < 1 || (v != reg.current && v != reg.current-1) {
		return nil, errs.New(errs.CodeDecodeFailed, op, attrs...)
	}
	var head struct {
		V *int `json:"v"`
	}
	if err := json.Unmarshal(payload, &head); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, attrs...)
	}
	if head.V == nil || *head.V != v {
		return nil, errs.New(errs.CodeDecodeFailed, op, attrs...)
	}
	return reg.decode(payload, attrs)
}
