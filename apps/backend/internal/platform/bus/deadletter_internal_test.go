package bus

import (
	"context"
	"slices"
	"testing"

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
