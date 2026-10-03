package scenario

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgainst_sendsEachScenariosClientIPAsTheForwardedAddress(t *testing.T) {
	t.Parallel()
	forwarded := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		forwarded <- r.Header.Get("X-Forwarded-For")
	}))
	t.Cleanup(srv.Close)
	for _, ip := range []string{"10.1.2.3", "10.4.5.6", ""} {
		Against(t.Context(), t, Remote{
			URL: srv.URL, ClientIP: ip, Enter: func(Stage) {}, Exchanged: func(Exchange) {},
		}).
			When(Post("/v1/system/pings", "{}"))
		if got := <-forwarded; got != ip {
			t.Fatalf("X-Forwarded-For = %q, want %q", got, ip)
		}
	}
}

func TestAgainst_scopesTheFlowHeaderToTheTriggeredRoute(t *testing.T) {
	t.Parallel()
	headers := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("X-Monaco-Flow")
	}))
	t.Cleanup(srv.Close)
	s := Against(t.Context(), t, Remote{
		URL: srv.URL, Flow: "02", Trigger: "POST /v1/cabals/{id}", Enter: func(Stage) {}, Exchanged: func(Exchange) {},
	})
	s.Given(Post("/v1/auth/session", "{}"))
	s.When(Post("/v1/cabals/c-1", "{}"))
	if got := <-headers; got != "" {
		t.Fatalf("setup flow header = %q, want empty", got)
	}
	if got := <-headers; got != "02" {
		t.Fatalf("trigger flow header = %q, want 02", got)
	}
}

func TestAgainst_reopensStreamsAfterAFaultpointRestart(t *testing.T) {
	t.Parallel()
	opened := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/session":
			_, _ = w.Write([]byte(`{"id":"01890a5d-ac96-774b-bcce-b302099a8058"}`))
		case "/v1/stream":
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			opened <- struct{}{}
			<-r.Context().Done()
		case "/crash":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"faultpoint"}`))
		}
	}))
	t.Cleanup(srv.Close)
	restarted := 0
	s := Against(t.Context(), t, Remote{
		URL: srv.URL, PrivyAppID: "verify", Mint: func(string) string { return "token" },
		Enter: func(Stage) {}, Exchanged: func(Exchange) {},
		Restart: func(context.Context) error { restarted++; return nil },
	})
	s.When(SignIn("did:privy:stream"))
	<-opened
	s.When(Post("/crash", "{}"), ExpectStatus(http.StatusServiceUnavailable))
	<-opened
	if restarted != 1 || s.Faults() != 1 {
		t.Fatalf("restart and faults = %d, %d, want 1, 1", restarted, s.Faults())
	}
}
