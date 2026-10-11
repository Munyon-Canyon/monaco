package testkit_test

import (
	"bytes"
	"reflect"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLogsKeepsEveryConcurrentWrite(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := logs.Write([]byte("line\n")); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := bytes.Count(logs.Bytes(), []byte("line\n")); n != 50 {
		t.Fatalf("Bytes holds %d lines after 50 concurrent writes", n)
	}
}

func TestLogsLinesKeepsOnlyTheNamedJSONMessages(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	_, _ = logs.Write([]byte(`{"msg":"a","n":1}` + "\nnot json\n" + `{"msg":"b"}` + "\n" + `{"msg":"a","n":2}` + "\n"))
	want := []string{"a 1", "a 2"}
	if got := logs.Lines("a", "msg", "n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Lines(a) = %q, want %q", got, want)
	}
}
