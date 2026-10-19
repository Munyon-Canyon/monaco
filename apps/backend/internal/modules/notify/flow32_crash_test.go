//go:build faultpoints

package notify_test

import (
	"bytes"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func crashFirstPreferencesPut(point faultpoint.Name) func(http.Handler) http.Handler {
	var armed atomic.Bool
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/notification-preferences") &&
				armed.CompareAndSwap(false, true) {
				r = r.WithContext(faultpoint.Armed(r.Context(), point))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TestFlow32_SetNotificationPreferences_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	flows.F32SetNotificationPreferencesCrashBeforeCommit(flow32Scenario(t,
		scenario.WithLogs(logs), scenario.WithRequestMiddleware(crashFirstPreferencesPut(faultpoint.BeforeCommit))))
	if !bytes.Contains(logs.Bytes(), []byte("faultpoint: crash at before-commit")) {
		t.Fatalf("the first save never crashed at before-commit; logs:\n%s", logs.Bytes())
	}
}
