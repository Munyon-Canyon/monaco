package fakes_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const ablyChannel = "cabal:0190a5d0-0000-7000-8000-000000000001"

func ablyServer(t *testing.T) (*fakes.Server, string) {
	t.Helper()
	srv := fakes.New()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts.URL
}

func ablyPost(t *testing.T, base, channel, body string, authorized bool) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		base+"/ably/channels/"+channel+"/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if authorized {
		req.Header.Set("Authorization", "Basic a2V5")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(got)
}

func TestAblyPublish_recordsWhatWasPublishedAndServesTheOkFixture(t *testing.T) {
	t.Parallel()
	srv, base := ablyServer(t)
	body := `[{"name":"message.created","data":"{\"id\":\"m1\"}","encoding":"json"},{"name":"plain","data":"hi"}]`
	status, reply := ablyPost(t, base, ablyChannel, body, true)
	if status != http.StatusCreated || !strings.Contains(reply, "serials") {
		t.Fatalf("publish = %d %q, want 201 with serials", status, reply)
	}
	want := []fakes.AblyPublish{
		{Channel: ablyChannel, Name: "message.created", Data: map[string]any{"id": "m1"}},
		{Channel: ablyChannel, Name: "plain", Data: "hi"},
	}
	if got := srv.AblyPublishes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("published = %+v, want %+v", got, want)
	}
}

func TestAblyPublish_acceptsASingleMessageObject(t *testing.T) {
	t.Parallel()
	srv, base := ablyServer(t)
	if status, _ := ablyPost(t, base, ablyChannel, `{"name":"x","data":"y"}`, true); status != http.StatusCreated {
		t.Fatalf("publish = %d, want 201", status)
	}
	if got := srv.AblyPublishes(); len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("published = %+v, want one x", got)
	}
}

func TestAblyPublish_refusesWhatAblyWouldRefuse(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body       string
		authorized bool
		status     int
	}{
		"no credentials":    {`{"name":"x","data":"y"}`, false, http.StatusUnauthorized},
		"not json":          {`{`, true, http.StatusBadRequest},
		"not a message":     {`7`, true, http.StatusBadRequest},
		"no name":           {`[{"data":"y"}]`, true, http.StatusBadRequest},
		"json that is not":  {`[{"name":"x","data":"{","encoding":"json"}]`, true, http.StatusBadRequest},
		"json data not str": {`[{"name":"x","data":{"a":1},"encoding":"json"}]`, true, http.StatusBadRequest},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, base := ablyServer(t)
			if status, reply := ablyPost(t, base, ablyChannel, tt.body, tt.authorized); status != tt.status {
				t.Fatalf("publish %s = %d %q, want %d", tt.body, status, reply, tt.status)
			}
			if n := len(srv.AblyPublishes()); n != 0 {
				t.Fatalf("a refused publish left %d messages behind", n)
			}
		})
	}
}

func TestAblyPublish_aScriptOnTheMessagesRouteFailsEveryChannelThenRecovers(t *testing.T) {
	t.Parallel()
	srv, base := ablyServer(t)
	step, err := json.Marshal(fakes.Step{
		Route: fakes.AblyMessagesRun, Action: fakes.ActionFail, Status: http.StatusInternalServerError, Times: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, base+"/_script", strings.NewReader(string(step)))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	body := `{"name":"x","data":"y"}`
	first, _ := ablyPost(t, base, ablyChannel, body, true)
	second, _ := ablyPost(t, base, "cabal:other", body, true)
	if first != http.StatusInternalServerError || second != http.StatusCreated || len(srv.AblyPublishes()) != 1 {
		t.Fatalf("statuses = %d, %d with %d published, want 500, 201 and one published",
			first, second, len(srv.AblyPublishes()))
	}
}

func ablyControl(t *testing.T, base, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/_ably", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestAblyExpect_countsThePublishesOfOneEventOnOneChannel(t *testing.T) {
	t.Parallel()
	_, base := ablyServer(t)
	for _, channel := range []string{ablyChannel, ablyChannel, "cabal:other"} {
		body := `{"name":"message.created","data":"y"}`
		if status, _ := ablyPost(t, base, channel, body, true); status != http.StatusCreated {
			t.Fatalf("publish = %d, want 201", status)
		}
	}
	want := `{"channel":"` + ablyChannel + `","event":"message.created","count":%d}`
	tests := map[string]struct {
		body   string
		status int
	}{
		"the right count": {strings.Replace(want, "%d", "2", 1), http.StatusNoContent},
		"too few":         {strings.Replace(want, "%d", "1", 1), http.StatusConflict},
		"too many":        {strings.Replace(want, "%d", "3", 1), http.StatusConflict},
		"another event": {
			`{"channel":"` + ablyChannel + `","event":"message.deleted","count":0}`, http.StatusNoContent,
		},
		"an unknown field":        {`{"count":1,"extra":1}`, http.StatusBadRequest},
		"a body that is not json": {`{`, http.StatusBadRequest},
	}
	for name, tt := range tests {
		if got := ablyControl(t, base, tt.body); got != tt.status {
			t.Errorf("%s: POST /_ably %s = %d, want %d", name, tt.body, got, tt.status)
		}
	}
}
