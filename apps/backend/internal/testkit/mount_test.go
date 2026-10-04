package testkit_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestServes_reportsWhetherTheMountRegisteredTheRoute(t *testing.T) {
	t.Parallel()
	mount := func(m api.Mount) { m.Mux.HandleFunc("GET /v1/x/{id}", func(http.ResponseWriter, *http.Request) {}) }
	if !testkit.Serves(mount, http.MethodGet, "/v1/x/1") {
		t.Fatal("Serves(GET /v1/x/1) = false, want true")
	}
	if testkit.Serves(mount, http.MethodPost, "/v1/x/1") || testkit.Serves(mount, http.MethodGet, "/v1/y") {
		t.Fatal("Serves reports a route the mount did not register")
	}
}
