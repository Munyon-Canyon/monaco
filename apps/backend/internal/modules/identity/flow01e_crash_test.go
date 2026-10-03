//go:build faultpoints

package identity_test

import (
	"bytes"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func crashFirstDelete(point faultpoint.Name) func(http.Handler) http.Handler {
	var armed atomic.Bool
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/v1/me" && armed.CompareAndSwap(false, true) {
				r = r.WithContext(faultpoint.Armed(r.Context(), point))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TestFlow01e_DeleteAccount_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	flows.F01eDeleteAccountCrashBeforeCommit(deleteScenario(t, fakes.NewTreasury(), fakes.NewBalances(),
		scenario.WithLogs(logs), scenario.WithRequestMiddleware(crashFirstDelete(faultpoint.BeforeCommit))))
	if !bytes.Contains(logs.Bytes(), []byte("faultpoint: crash at before-commit")) {
		t.Fatalf("the first delete never crashed at before-commit; logs:\n%s", logs.Bytes())
	}
}
