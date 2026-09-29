package app

import (
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	t.Parallel()
	<-time.After(time.Millisecond)
}
