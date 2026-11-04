//go:build faultpoints

package social_test

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

func crashFirstChatPin(point faultpoint.Name) func(http.Handler) http.Handler {
	var armed atomic.Bool
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/pin") &&
				armed.CompareAndSwap(false, true) {
				r = r.WithContext(faultpoint.Armed(r.Context(), point))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TestFlow22f_PinChatMessage_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	flows.F22fPinChatMessageCrashBeforeCommit(chatScenario(t,
		scenario.WithLogs(logs), scenario.WithRequestMiddleware(crashFirstChatPin(faultpoint.BeforeCommit))))
	if !bytes.Contains(logs.Bytes(), []byte("faultpoint: crash at before-commit")) {
		t.Fatalf("the first chat pin never crashed at before-commit; logs:\n%s", logs.Bytes())
	}
}
