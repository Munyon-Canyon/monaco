package main

import (
	"io"
	"testing"
)

func TestToolMarketing_unknownCommandExitsTwo(t *testing.T) {
	t.Parallel()
	if code := toolMarketing(toolEnv{})([]string{"nope"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
