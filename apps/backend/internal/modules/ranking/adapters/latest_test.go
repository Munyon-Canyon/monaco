package adapters_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLatest_failsWhenTheDatabaseCallFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	latest := adapters.Latest{DB: testkit.DB(t)}
	if _, err := latest.LatestRun(ctx); err == nil {
		t.Fatal("LatestRun() error = nil")
	}
	if _, err := latest.LatestCabalValues(ctx); err == nil {
		t.Fatal("LatestCabalValues() error = nil")
	}
}
