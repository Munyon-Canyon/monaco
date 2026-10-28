package scenario

import "net/http"

func UntilRateLimited(attempts int, send Step) Step {
	return func(s *Scenario) {
		s.t.Helper()
		for range attempts {
			send(s)
			if s.response().status == http.StatusTooManyRequests {
				return
			}
		}
		s.t.Fatalf("scenario: %d requests in a row were not rate limited; the last answered %d %s",
			attempts, s.response().status, s.response().body)
	}
}
