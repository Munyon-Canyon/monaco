package testkit

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RealtimePublish struct {
	Channel string
	Name    string
	Data    json.RawMessage
}

type FakeRealtime struct {
	faults    Faults
	mu        sync.Mutex
	published []RealtimePublish
}

var _ app.Realtime = (*FakeRealtime)(nil)

func (f *FakeRealtime) Fail(err error) { f.faults.Fail("Publish", err) }

func (f *FakeRealtime) FailOnce(err error) { f.faults.FailOnce("Publish", err) }

func (f *FakeRealtime) Publish(_ context.Context, channel string, name string, data any) error {
	if err := f.faults.Check("Publish"); err != nil {
		return err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "testkit.FakeRealtime.Publish")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, RealtimePublish{Channel: channel, Name: name, Data: raw})
	return nil
}

func (f *FakeRealtime) Published() []RealtimePublish {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.published)
}

func (f *FakeRealtime) TokenRequest(
	_ context.Context, clientID ids.UserID, channels []string, ttl time.Duration,
) (app.TokenRequest, error) {
	capability := make(map[string][]string, len(channels))
	for _, channel := range channels {
		capability[channel] = []string{"subscribe"}
	}
	raw, _ := json.Marshal(capability)
	return app.TokenRequest{
		KeyName: "fake.key", ClientID: clientID.String(), Capability: string(raw), TTL: ttl.Milliseconds(),
		Timestamp: 1, Nonce: "fake-nonce", MAC: "fake-mac",
	}, nil
}
