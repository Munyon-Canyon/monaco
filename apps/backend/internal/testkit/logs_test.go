package testkit_test

import (
	"bytes"
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
