package posthogfake_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/posthogfake"
)

func post(t *testing.T, url string, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestFake_recordsWhatIsPostedToItsHostAndFailsOnScript(t *testing.T) {
	t.Parallel()
	fake := posthogfake.New(t)
	fake.Fail(t, http.StatusServiceUnavailable, 1)
	body := []byte(`{"api_key":"k","batch":[{"uuid":"0190a5d0-0000-7000-8000-000000000001","event":"e",` +
		`"distinct_id":"u","timestamp":"` + clock.Real{}.Now().UTC().Format(time.RFC3339) + `"}]}`)
	first := post(t, fake.Host()+"/batch/", body)
	second := post(t, fake.Host()+"/batch/", body)
	if first != http.StatusServiceUnavailable || second != http.StatusOK || len(fake.Captures()) != 1 ||
		fake.Received() != 1 {
		t.Fatalf("statuses %d then %d with %d captures and %d received, want 503, 200, one and one",
			first, second, len(fake.Captures()), fake.Received())
	}
}
