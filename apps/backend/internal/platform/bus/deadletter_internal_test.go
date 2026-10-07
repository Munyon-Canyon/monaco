package bus

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type failingGetter struct{ err error }

func (f failingGetter) GetMsg(context.Context, uint64, ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	return nil, f.err
}

func TestReadDeadLetters_skipsDeletedLettersAndFailsOnAnyOtherError(t *testing.T) {
	t.Parallel()
	letters, err := readDeadLetters(t.Context(), failingGetter{jetstream.ErrMsgNotFound}, 1, 3)
	if err != nil || len(letters) != 0 {
		t.Fatalf("all deleted = %v, %v, want none", letters, err)
	}
	_, err = readDeadLetters(t.Context(), failingGetter{jetstream.ErrNoStreamResponse}, 1, 1)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}

type recordingGetter struct{ asked []uint64 }

func (r *recordingGetter) GetMsg(
	_ context.Context,
	seq uint64,
	_ ...jetstream.GetMsgOpt,
) (*jetstream.RawStreamMsg, error) {
	r.asked = append(r.asked, seq)
	return &jetstream.RawStreamMsg{Data: []byte(`{}`)}, nil
}

func TestReadDeadLetters_readsExactlyFirstThroughLastAndNothingFromAnEmptyStream(t *testing.T) {
	t.Parallel()
	g := &recordingGetter{}
	letters, err := readDeadLetters(t.Context(), g, 3, 5)
	if err != nil || !slices.Equal(g.asked, []uint64{3, 4, 5}) || len(letters) != 3 ||
		letters[0].Seq != 3 || letters[2].Seq != 5 {
		t.Fatalf("letters = %+v, %v after asking for %v, want sequences 3, 4 and 5 in order", letters, err, g.asked)
	}
	empty := &recordingGetter{}
	letters, err = readDeadLetters(t.Context(), empty, 0, 0)
	if err != nil || len(letters) != 0 || len(empty.asked) != 0 {
		t.Fatalf("empty stream = %+v, %v after asking for %v, want no reads", letters, err, empty.asked)
	}
}

type pulledMsg struct {
	jetstream.Msg
	data    []byte
	meta    *jetstream.MsgMetadata
	metaErr error
	ackErr  error
	verdict string
}

func (m *pulledMsg) Data() []byte                              { return m.data }
func (m *pulledMsg) Metadata() (*jetstream.MsgMetadata, error) { return m.meta, m.metaErr }
func (m *pulledMsg) Ack() error                                { m.verdict = "ack"; return m.ackErr }
func (m *pulledMsg) Nak() error                                { m.verdict = "nak"; return nil }
func (m *pulledMsg) Term() error                               { m.verdict = "term"; return m.ackErr }

func TestPulled_termsAnUndecodableLetterAndFailsOneWithoutMetadataLeavingItUnanswered(t *testing.T) {
	t.Parallel()
	noop := func(context.Context, DeadLetter) error { return nil }
	garbage := &pulledMsg{data: []byte("{")}
	if err := pulled(t.Context(), garbage, noop); err != nil || garbage.verdict != "term" {
		t.Fatalf("garbage = %q, %v, want term and no error", garbage.verdict, err)
	}
	blind := &pulledMsg{data: []byte("{}"), metaErr: errors.New("no reply subject")}
	if err := pulled(t.Context(), blind, noop); errs.CodeOf(err) != errs.CodeUpstreamUnavailable ||
		blind.verdict != "" {
		t.Fatalf("without metadata = %q, %v, want no verdict and upstream_unavailable", blind.verdict, err)
	}
}

func TestPulled_reportsAnAckOrTermThatTheServerRefused(t *testing.T) {
	t.Parallel()
	noop := func(context.Context, DeadLetter) error { return nil }
	for _, data := range []string{"{", "{}"} {
		msg := &pulledMsg{data: []byte(data), meta: &jetstream.MsgMetadata{}, ackErr: errors.New("timeout")}
		if err := pulled(t.Context(), msg, noop); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
			t.Fatalf("%s with a refused %s = %v, want upstream_unavailable", data, msg.verdict, err)
		}
	}
}

type endedBatch struct {
	msgs chan jetstream.Msg
	err  error
}

func (b endedBatch) Messages() <-chan jetstream.Msg { return b.msgs }
func (b endedBatch) Error() error                   { return b.err }

func fetching(batch jetstream.MessageBatch, err error) fetchFunc {
	return func(int, ...jetstream.FetchOpt) (jetstream.MessageBatch, error) { return batch, err }
}

func TestDrain_reportsAFetchThatFailedAndABatchThatEndedWithAnError(t *testing.T) {
	t.Parallel()
	noop := func(context.Context, DeadLetter) error { return nil }
	n, err := drain(t.Context(), fetching(nil, errors.New("no responders")), 10, noop)
	if n != 0 || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("failed fetch = %d, %v, want upstream_unavailable", n, err)
	}
	msgs := make(chan jetstream.Msg, 1)
	msgs <- &pulledMsg{data: []byte("{}"), meta: &jetstream.MsgMetadata{}}
	close(msgs)
	batch := endedBatch{msgs: msgs, err: errors.New("server gone")}
	n, err = drain(t.Context(), fetching(batch, nil), 10, noop)
	if n != 1 || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("drain = %d, %v, want 1 message and upstream_unavailable", n, err)
	}
}

func TestDeadLetterEventID_prefersTheRedriveHeaderOverTheStreamMessageID(t *testing.T) {
	t.Parallel()
	letter := DeadLetter{Headers: nats.Header{jetstream.MsgIDHeader: []string{"event/redrive-1"}}}
	if got := letter.EventID(); got != "event/redrive-1" {
		t.Fatalf("EventID = %q, want the message id when nothing else names the event", got)
	}
	letter.Headers.Set(EventIDHeader, "event")
	if got := letter.EventID(); got != "event" {
		t.Fatalf("EventID = %q, want the Monaco-Event-Id header", got)
	}
}
