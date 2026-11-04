package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

type letterKey struct{ consumer, subject, code string }

func (k letterKey) String() string {
	consumer := k.consumer
	if consumer == "" {
		consumer = "?"
	}
	return consumer + "/" + k.code + "@" + k.subject
}

func (d *driver) expectedLetterKeys(units []Unit) []letterKey {
	var keys []letterKey
	for _, u := range units {
		if trigger, code, ok := termOf(u); ok {
			keys = append(keys, letterKey{"", d.env.Subject(events.Type(trigger).Subject()), string(code)})
		}
		for _, l := range flows.LettersOf(u.Script) {
			keys = append(keys, letterKey{l.Consumer, d.env.Subject(l.Subject.Subject()), string(l.Code)})
		}
	}
	return keys
}

func actualLetterKeys(ctx context.Context, stream jetstream.Stream) ([]letterKey, error) {
	state := stream.CachedInfo().State
	var keys []letterKey
	for seq := state.FirstSeq; seq <= state.LastSeq && seq > 0; seq++ {
		raw, err := stream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read dead letter %d: %w", seq, err)
		}
		var letter bus.DeadLetter
		_ = json.Unmarshal(raw.Data, &letter)
		consumer := letter.Consumer
		if consumer == "" {
			consumer = raw.Subject[strings.LastIndex(raw.Subject, ".")+1:]
		}
		keys = append(keys, letterKey{consumer, letter.Subject, letter.Code})
	}
	return keys, nil
}

func (d *driver) letterFailure(ctx context.Context, stream jetstream.Stream, units []Unit) error {
	got, err := actualLetterKeys(ctx, stream)
	if err != nil {
		return fmt.Errorf("read %s: %w", d.env.DeadLetter, err)
	}
	return lettersMismatch(d.expectedLetterKeys(units), got)
}

func lettersDiffer(want, got []letterKey) bool {
	rest := slices.Clone(got)
	var open []letterKey
	for _, w := range want {
		if i := slices.Index(rest, w); i >= 0 {
			rest = slices.Delete(rest, i, i+1)
		} else {
			open = append(open, w)
		}
	}
	for _, w := range open {
		i := slices.IndexFunc(rest, func(g letterKey) bool {
			return w.consumer == "" && g.subject == w.subject && g.code == w.code
		})
		if i < 0 {
			return true
		}
		rest = slices.Delete(rest, i, i+1)
	}
	return len(rest) > 0
}

func lettersMismatch(want, got []letterKey) error {
	if !lettersDiffer(want, got) {
		return nil
	}
	list := func(keys []letterKey) string {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = k.String()
		}
		slices.Sort(names)
		return "[" + strings.Join(names, " ") + "]"
	}
	return &InvariantError{Msg: fmt.Sprintf("dead letters differ: want %s, got %s", list(want), list(got))}
}
