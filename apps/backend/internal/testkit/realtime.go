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
	faults      Faults
	mu          sync.Mutex
	published   []RealtimePublish
	clients     []*RealtimeClient
	revocations []ids.UserID
	revokedAt   map[string]int
	connections int
}

type RealtimeClient struct {
	rt       *FakeRealtime
	clientID string
	channels map[string][]string
	serial   int
	received []RealtimePublish
}

var _ app.Realtime = (*FakeRealtime)(nil)

func (f *FakeRealtime) Fail(err error) { f.faults.Fail("Publish", err) }

func (f *FakeRealtime) FailOnce(err error) { f.faults.FailOnce("Publish", err) }

func (f *FakeRealtime) FailRevoke(err error) { f.faults.Fail("RevokeTokens", err) }

func (f *FakeRealtime) FailRevokeOnce(err error) { f.faults.FailOnce("RevokeTokens", err) }

func (f *FakeRealtime) Connect(req app.TokenRequest) (*RealtimeClient, error) {
	var capability map[string][]string
	if err := json.Unmarshal([]byte(req.Capability), &capability); err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "testkit.FakeRealtime.Connect")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connections++
	c := &RealtimeClient{rt: f, clientID: req.ClientID, channels: capability, serial: f.connections}
	f.clients = append(f.clients, c)
	return c, nil
}

func (f *FakeRealtime) Revocations() []ids.UserID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.revocations)
}

func (f *FakeRealtime) RevokeTokens(_ context.Context, clientID ids.UserID) error {
	if err := f.faults.Check("RevokeTokens"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connections++
	if f.revokedAt == nil {
		f.revokedAt = map[string]int{}
	}
	f.revokedAt[clientID.String()] = f.connections
	f.revocations = append(f.revocations, clientID)
	return nil
}

func (c *RealtimeClient) Received() []RealtimePublish {
	c.rt.mu.Lock()
	defer c.rt.mu.Unlock()
	return slices.Clone(c.received)
}

func (c *RealtimeClient) revoked() bool {
	at, ok := c.rt.revokedAt[c.clientID]
	return ok && c.serial < at
}

func (c *RealtimeClient) hears(channel string) bool {
	return slices.Contains(c.channels[channel], "subscribe") && !c.revoked()
}

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
	msg := RealtimePublish{Channel: channel, Name: name, Data: raw}
	f.published = append(f.published, msg)
	for _, c := range f.clients {
		if c.hears(channel) {
			c.received = append(c.received, msg)
		}
	}
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
