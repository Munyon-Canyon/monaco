package eventlog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
)

type Options struct {
	AggregateType string
	AggregateID   uuid.UUID
	Anonymize     bool
}

type Line struct {
	ID        uuid.UUID       `json:"id"`
	Type      events.Type     `json:"type"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

func Export(ctx context.Context, pool *pgxpool.Pool, w io.Writer, o Options) error {
	const op = "eventlog.Export"
	rows, _ := pool.Query(ctx, `SELECT id, type, actor_type || ':' || actor_id, created_at, payload FROM events
		WHERE $1 = '' OR (aggregate_type = $1 AND aggregate_id = $2) ORDER BY id`, o.AggregateType, o.AggregateID)
	lines, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Line])
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	enc := json.NewEncoder(w)
	for _, l := range lines {
		l.CreatedAt = l.CreatedAt.UTC()
		if o.Anonymize {
			if l, err = anonymize(l); err != nil {
				return err
			}
		}
		if err := enc.Encode(l); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	return nil
}

func anonymize(l Line) (Line, error) {
	var head struct {
		V int `json:"v"`
	}
	_ = json.Unmarshal(l.Payload, &head)
	ev, err := events.Decode(l.Type, head.V, l.Payload)
	if err != nil {
		return l, err
	}
	return anonymizeEvent(l, ev)
}

func anonymizeEvent(l Line, ev any) (Line, error) {
	actorType, actorID, _ := strings.Cut(l.Actor, ":")
	l.Actor = actorType + ":" + Pseudonym(actorID)
	var fields map[string]any
	_ = json.Unmarshal(l.Payload, &fields)
	scrub(reflect.ValueOf(ev), fields)
	l.Payload, _ = json.Marshal(fields)
	return l, nil
}

func scrub(v reflect.Value, fields map[string]any) {
	v = follow(v)
	if v.Kind() != reflect.Struct {
		return
	}
	for i := range v.NumField() {
		f := v.Type().Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		child, ok := fields[name]
		if !ok {
			continue
		}
		switch f.Tag.Get("pii") {
		case "true":
			fields[name] = pseudonymize(child)
			continue
		case "keys":
			hashed, ok := hashKeys(child)
			if !ok {
				continue
			}
			fields[name] = hashed
			scrubMap(follow(v.Field(i)), hashed)
			continue
		}
		scrubOne(v.Field(i), child)
	}
}

func hashKeys(child any) (map[string]any, bool) {
	obj, ok := child.(map[string]any)
	if !ok {
		return nil, false
	}
	rebuilt := make(map[string]any, len(obj))
	for key, val := range obj {
		rebuilt[Pseudonym(key)] = val
	}
	return rebuilt, true
}

func follow(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Zero(derefType(v.Type()))
		}
		v = v.Elem()
	}
	return v
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func scrubOne(v reflect.Value, child any) {
	v = follow(v)
	switch {
	case v.Kind() == reflect.Struct:
		nested, ok := child.(map[string]any)
		if !ok {
			return
		}
		scrub(v, nested)
	case v.Kind() == reflect.Map:
		scrubMap(v, child)
	case v.Kind() == reflect.Interface && !v.IsNil():
		scrubOne(v.Elem(), child)
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		scrubIndexed(v, child)
	}
}

func scrubIndexed(v reflect.Value, child any) {
	elem := derefType(v.Type().Elem())
	if elem.Kind() != reflect.Struct && elem.Kind() != reflect.Map &&
		elem.Kind() != reflect.Slice && elem.Kind() != reflect.Array {
		return
	}
	list, ok := child.([]any)
	if !ok {
		return
	}
	for i, item := range list {
		cur := reflect.Zero(v.Type().Elem())
		if i < v.Len() {
			cur = v.Index(i)
		}
		scrubOne(cur, item)
	}
}

func scrubMap(v reflect.Value, child any) {
	obj, ok := child.(map[string]any)
	if !ok {
		return
	}
	for _, val := range obj {
		scrubOne(reflect.Zero(v.Type().Elem()), val)
	}
}

func pseudonymize(node any) any {
	list, isList := node.([]any)
	if !isList {
		return Pseudonym(fmt.Sprint(node))
	}
	out := make([]any, len(list))
	for i, element := range list {
		out[i] = Pseudonym(fmt.Sprint(element))
	}
	return out
}

func Pseudonym(s string) string {
	if _, err := uuid.Parse(s); err == nil {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte("monaco:"+s)).String()
	}
	sum := sha256.Sum256([]byte("monaco:" + s))
	return "anon-" + hex.EncodeToString(sum[:8])
}
