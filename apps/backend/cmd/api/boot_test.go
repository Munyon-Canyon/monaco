package main

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestNewHandler_rejectsPartialStorageConfig(t *testing.T) {
	t.Parallel()
	deps := module.Deps{Config: config.Config{Supabase: config.Supabase{URL: "https://storage.example"}}}
	_, err := newHandler(deps, nil, nil)
	if errs.CodeOf(err) != errs.CodeInvalidConfig {
		t.Fatalf("newHandler error = %v", err)
	}
}
