package scenario

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSignIn_aRefusedSignInLeavesTheScenarioSignedOut(t *testing.T) {
	t.Parallel()
	var signIns atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/stream":
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			<-r.Context().Done()
		case signIns.Add(1) == 1:
			_, _ = w.Write([]byte(`{"id":"01890a5d-ac96-774b-bcce-b302099a8058"}`))
		default:
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated"}`))
		}
	}))
	t.Cleanup(srv.Close)
	s := Against(t.Context(), t, Remote{
		URL: srv.URL, PrivyAppID: "verify", Mint: func(string) string { return "token" },
		Enter: func(Stage) {}, Exchanged: func(Exchange) {},
	})
	s.When(SignIn("did:privy:first"))
	if s.actor == nil {
		t.Fatal("no actor after a 200 sign-in")
	}
	s.When(SignIn("did:privy:second"), ExpectStatus(http.StatusUnauthorized))
	if s.actor != nil {
		t.Fatalf("actor %q still signed in after a refused sign-in, want signed out", s.actor.token)
	}
}
