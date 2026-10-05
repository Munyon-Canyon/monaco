package bus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
)

const (
	StreamEvents     = "EVENTS"
	StreamDeadLetter = "DEADLETTER"
)

func Streams() []jetstream.StreamConfig {
	return []jetstream.StreamConfig{
		{
			Name:       StreamEvents,
			Subjects:   events.Subjects(),
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			MaxBytes:   2 << 30,
			MaxAge:     7 * 24 * time.Hour,
			Discard:    jetstream.DiscardNew,
			Duplicates: 2 * time.Minute,
		},
		{
			Name:       StreamDeadLetter,
			Subjects:   []string{"deadletter.>"},
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			MaxBytes:   512 << 20,
			MaxAge:     30 * 24 * time.Hour,
			Discard:    jetstream.DiscardOld,
			Duplicates: 2 * time.Minute,
		},
	}
}

type Action string

const (
	ActionCreated   Action = "created"
	ActionUpdated   Action = "updated"
	ActionUnchanged Action = "no changes"
)

type StreamChange struct {
	Stream string
	Action Action
	Fields []FieldChange
}

type FieldChange struct {
	Field    string
	From, To string
}

func (c StreamChange) String() string {
	var b strings.Builder
	b.WriteString(c.Stream + ": " + string(c.Action) + "\n")
	for _, f := range c.Fields {
		b.WriteString("  " + f.Field + ": " + f.From + " -> " + f.To + "\n")
	}
	return b.String()
}

func (c *Conn) Apply(ctx context.Context) ([]StreamChange, error) {
	const op = "bus.Apply"
	changes := make([]StreamChange, 0, len(Streams()))
	for _, want := range c.streams() {
		attrs := []slog.Attr{slog.String("stream", want.Name)}
		s, err := c.js.Stream(ctx, want.Name)
		if errors.Is(err, jetstream.ErrStreamNotFound) {
			if _, err := c.js.CreateStream(ctx, want); err != nil {
				return changes, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
			}
			changes = append(changes, StreamChange{Stream: want.Name, Action: ActionCreated})
			continue
		}
		if err != nil {
			return changes, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
		}
		diff := diffStream(s.CachedInfo().Config, want)
		if len(diff) == 0 {
			changes = append(changes, StreamChange{Stream: want.Name, Action: ActionUnchanged})
			continue
		}
		if _, err := c.js.UpdateStream(ctx, want); err != nil {
			return changes, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
		}
		changes = append(changes, StreamChange{Stream: want.Name, Action: ActionUpdated, Fields: diff})
	}
	return changes, nil
}

var errStreamDrift = errors.New("config differs from the declared one")

func (c *Conn) VerifyStreams(ctx context.Context) error {
	const op = "bus.VerifyStreams"
	for _, want := range c.streams() {
		attr := slog.String("stream", want.Name)
		s, err := c.js.Stream(ctx, want.Name)
		if err != nil {
			code := errs.CodeUpstreamUnavailable
			if errors.Is(err, jetstream.ErrStreamNotFound) {
				code = errs.CodeNotFound
			}
			return errs.Wrap(fmt.Errorf("stream %s: %w; run monacoctl bus apply", want.Name, err), code, op, attr)
		}
		if diff := diffStream(s.CachedInfo().Config, want); len(diff) > 0 {
			fields := make([]string, len(diff))
			for i, f := range diff {
				fields[i] = f.Field
			}
			err := fmt.Errorf("stream %s: %w in %s; run monacoctl bus apply",
				want.Name, errStreamDrift, strings.Join(fields, ", "))
			return errs.Wrap(err, errs.CodeNotFound, op, attr)
		}
	}
	return nil
}

func (c *Conn) streams() []jetstream.StreamConfig {
	out := Streams()
	for i := range out {
		out[i].Name = c.ns.stream(out[i].Name)
		for j, s := range out[i].Subjects {
			out[i].Subjects[j] = c.ns.subject(s)
		}
	}
	return out
}

type streamField struct {
	name   string
	render func(jetstream.StreamConfig) string
}

func streamFields() []streamField {
	return []streamField{
		{"Subjects", func(s jetstream.StreamConfig) string {
			return strings.Join(slices.Sorted(slices.Values(s.Subjects)), ",")
		}},
		{"Retention", func(s jetstream.StreamConfig) string { return s.Retention.String() }},
		{"Storage", func(s jetstream.StreamConfig) string { return s.Storage.String() }},
		{"Replicas", func(s jetstream.StreamConfig) string { return strconv.Itoa(s.Replicas) }},
		{"MaxBytes", func(s jetstream.StreamConfig) string { return strconv.FormatInt(s.MaxBytes, 10) }},
		{"MaxAge", func(s jetstream.StreamConfig) string { return s.MaxAge.String() }},
		{"Discard", func(s jetstream.StreamConfig) string { return s.Discard.String() }},
		{"Duplicates", func(s jetstream.StreamConfig) string { return s.Duplicates.String() }},
	}
}

func diffStream(have, want jetstream.StreamConfig) []FieldChange {
	var diff []FieldChange
	for _, f := range streamFields() {
		if from, to := f.render(have), f.render(want); from != to {
			diff = append(diff, FieldChange{Field: f.name, From: from, To: to})
		}
	}
	return diff
}
