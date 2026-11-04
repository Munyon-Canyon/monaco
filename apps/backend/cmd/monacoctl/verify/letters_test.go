package verify

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func TestLetterFailure_readsTheConsumerFromTheSubjectAndSkipsDeletedLetters(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	d, err := newDriver(env, testBudget())
	if err != nil {
		t.Fatal(err)
	}
	ns := strings.TrimSuffix(env.DeadLetter, "_"+bus.StreamDeadLetter)
	for _, body := range []string{`{"consumer":"gone","subject":"s","code":"c"}`, `{"subject":"s","code":"c"}`} {
		if _, err := env.JS.Publish(t.Context(), ns+".deadletter.notify", []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	stream, err := env.JS.Stream(t.Context(), env.DeadLetter)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.DeleteMsg(t.Context(), stream.CachedInfo().State.FirstSeq); err != nil {
		t.Fatal(err)
	}
	got, err := actualLetterKeys(t.Context(), stream)
	if want := []letterKey{{"notify", "s", "c"}}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("actualLetterKeys = %v, %v, want %v", got, err, want)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := d.letterFailure(ctx, stream, nil); err == nil || !strings.Contains(err.Error(), "read "+env.DeadLetter) {
		t.Fatalf("letterFailure on a canceled context = %v, want a read failure", err)
	}
}
