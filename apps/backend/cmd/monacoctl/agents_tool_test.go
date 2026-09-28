package main

import (
	"io"
	"testing"
)

func TestToolAgents_unknownCommandExitsTwo(t *testing.T) {
	t.Parallel()
	if code := toolAgents(toolEnv{})([]string{"nope"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
