package scenario

import (
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
		Against(t, Remote{URL: srv.URL, ClientIP: ip, Enter: func(Stage) {}, Exchanged: func(Exchange) {}}).
			When(Post("/v1/system/pings", "{}"))
		if got := <-forwarded; got != ip {
			t.Fatalf("X-Forwarded-For = %q, want %q", got, ip)
		}
	}
}
