package httpx_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAllocsProblemJSON(t *testing.T) {
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard)
	r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), logger), http.MethodGet, "/v1/x", nil)
	err := errs.New(errs.CodeInvalidInput, "test.op")
	testkit.AssertAllocs(t, "httpx.Problem invalid_input", func() {
		httpx.Problem(httptest.NewRecorder(), r, err)
	})
}
