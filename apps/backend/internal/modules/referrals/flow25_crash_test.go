//go:build faultpoints

package referrals_test

import (
	"bytes"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func crashFirstAttach(point faultpoint.Name) func(http.Handler) http.Handler {
	var armed atomic.Bool
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/v1/me/referral" && armed.CompareAndSwap(false, true) {
				r = r.WithContext(faultpoint.Armed(r.Context(), point))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TestFlow25_AttachReferral_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	flows.F25AttachReferralCrashBeforeCommit(referralScenario(t,
		scenario.WithLogs(logs), scenario.WithRequestMiddleware(crashFirstAttach(faultpoint.BeforeCommit))))
	if !bytes.Contains(logs.Bytes(), []byte("faultpoint: crash at before-commit")) {
		t.Fatalf("the first attach never crashed at before-commit; logs:\n%s", logs.Bytes())
	}
}
