package testkit

import (
	"context"
	"embed"
	"maps"
	"net/http"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

//go:embed testdata/apns/test_key.p8
var apnsKey embed.FS

func APNsKeyP8() string {
	raw, _ := apnsKey.ReadFile("testdata/apns/test_key.p8")
	return string(raw)
}

func APNsEnv() []string {
	return []string{"APNS_KEY_P8=" + APNsKeyP8(), "APNS_KEY_ID=TESTKEYID1", "APNS_TEAM_ID=TESTTEAMID"}
}

type FakeSender struct {
	Faults
	mu      sync.Mutex
	sent    []apns.Push
	replies map[string]apns.Result
}

var _ apns.Sender = (*FakeSender)(nil)

func (f *FakeSender) Send(_ context.Context, p apns.Push) (apns.Result, error) {
	p.Data = maps.Clone(p.Data)
	f.mu.Lock()
	f.sent = append(f.sent, p)
	reply, scripted := f.replies[p.Token]
	f.mu.Unlock()
	if err := f.Check("Send"); err != nil {
		return apns.Result{}, err
	}
	if scripted {
		return reply, nil
	}
	return apns.Result{Status: http.StatusOK}, nil
}

func (f *FakeSender) Reply(token string, r apns.Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.replies == nil {
		f.replies = map[string]apns.Result{}
	}
	f.replies[token] = r
}

func (f *FakeSender) Sent() []apns.Push {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}
