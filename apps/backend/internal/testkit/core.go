package testkit

import (
	"testing"

	"github.com/nats-io/nats.go"
)

type CoreSubscription struct {
	sub *nats.Subscription
}

func SubscribeCore(t *testing.T, b Bus, subject string) *CoreSubscription {
	t.Helper()
	s := natsCurrent.Load()
	if s == nil {
		t.Fatal("testkit.SubscribeCore: call testkit.Main(m, testkit.WithNATS()) from this package's TestMain")
	}
	sub, err := s.admin.SubscribeSync(b.Conn.Subject(subject))
	if err == nil {
		err = s.admin.Flush()
	}
	if err != nil {
		t.Fatalf("testkit.SubscribeCore %s: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return &CoreSubscription{sub: sub}
}

func (c *CoreSubscription) Next(t *testing.T) []byte {
	t.Helper()
	msg, err := c.sub.NextMsg(natsReady)
	if err != nil {
		t.Fatalf("testkit.CoreSubscription.Next on %s: %v", c.sub.Subject, err)
	}
	return msg.Data
}
