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
	actorType, actorID, _ := strings.Cut(l.Actor, ":")
	l.Actor = actorType + ":" + Pseudonym(actorID)
	var fields map[string]any
	_ = json.Unmarshal(l.Payload, &fields)
	scrub(reflect.TypeOf(ev), fields)
	l.Payload, _ = json.Marshal(fields)
	return l, nil
}

func scrub(t reflect.Type, fields map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		child, ok := fields[name]
		if !ok {
			continue
		}
		if f.Tag.Get("pii") == "true" {
			fields[name] = pseudonymize(child)
			continue
		}
		if nested, isObject := child.(map[string]any); isObject {
			scrub(f.Type, nested)
		}
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
