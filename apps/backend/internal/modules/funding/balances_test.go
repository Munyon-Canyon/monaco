package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule(t *testing.T) {
	t.Parallel()
	m := funding.New(module.Deps{})
	if got := m.Name(); got != "funding" {
		t.Fatalf("Name = %q, want funding", got)
	}
	m.Routes(&httpx.Routes{})
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
	if got := m.Pollers(); got != nil {
		t.Fatalf("Pollers = %v, want nil", got)
	}
	if m.Balances() == nil {
		t.Fatal("Balances = nil")
	}
}
