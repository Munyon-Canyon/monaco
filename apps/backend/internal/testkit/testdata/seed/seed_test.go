package seed_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFails(t *testing.T) {
	t.Errorf("seed=%d", testkit.RandSeed(t))
}

func TestPasses(t *testing.T) {
	t.Logf("seed=%d", testkit.RandSeed(t))
}
