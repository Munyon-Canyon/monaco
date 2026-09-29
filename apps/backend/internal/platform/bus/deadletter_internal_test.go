package bus

import (
	"context"
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
