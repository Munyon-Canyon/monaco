package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestNewHandler_rejectsPartialStorageConfig(t *testing.T) {
	t.Parallel()
	deps := module.Deps{Config: config.Config{Supabase: config.Supabase{URL: "https://storage.example"}}}
	_, err := newHandler(deps, nil, nil)
	if errs.CodeOf(err) != errs.CodeInvalidConfig {
		t.Fatalf("newHandler error = %v", err)
	}
}

func TestBoot_mountsEveryOperationInTheContract(t *testing.T) {
	t.Parallel()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registered.Build(module.Deps{Clock: clock.Real{}, Config: config.Config{
		Privy:    config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()},
		Timeouts: config.Timeouts{Privy: time.Second},
	}}).Mount(api.Mount{Mux: mux})
	param := regexp.MustCompile(`\{[^}]+\}`)
	var ops int
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			ops++
			target := param.ReplaceAllString(path, "p")
			_, pattern := mux.Handler(httptest.NewRequestWithContext(t.Context(), method, target, nil))
			if pattern == "" {
				t.Errorf("%s %s is in api/openapi.yaml but no module mounts it", method, path)
			}
		}
	}
	if ops == 0 {
		t.Fatal("api/openapi.yaml declares no operations")
	}
}
