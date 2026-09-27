package testkit

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestConsumerConfig_defaultsTo100msAndRejectsOver250ms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ackWait time.Duration
		ok      bool
	}{
		{DefaultAckWait, true},
		{MaxAckWait, true},
		{MaxAckWait + time.Millisecond, false},
		{time.Second, false},
		{0, false},
		{-time.Millisecond, false},
	} {
		cfg, err := consumerConfig(tc.ackWait)
		if tc.ok != (err == nil) {
			t.Fatalf("consumerConfig(%s) err = %v, want ok=%v", tc.ackWait, err, tc.ok)
		}
		if tc.ok && (cfg.AckWait != tc.ackWait || cfg.AckPolicy != jetstream.AckExplicitPolicy) {
			t.Fatalf("consumerConfig(%s) = %+v", tc.ackWait, cfg)
		}
		if !tc.ok && !strings.Contains(err.Error(), "outside (0, 250ms]") {
			t.Fatalf("consumerConfig(%s) err = %v, want the 250ms bound named", tc.ackWait, err)
		}
	}
}

func TestNATSNamespace_isAValidStreamAndSubjectTokenPerTest(t *testing.T) {
	t.Parallel()
	a, b := natsNamespace("TestX/sub case.with*odd>chars"), natsNamespace("TestX/sub case.with*odd>chars")
	if a == b {
		t.Fatalf("two namespaces for one name collide: %s", a)
	}
	if !strings.HasPrefix(a, "t_TestX_sub_case_with_odd_chars_") || strings.ContainsAny(a, " .*>/") {
		t.Fatalf("namespace %q is not a safe stream and subject token", a)
	}
}

func TestStartNATS_startsAndStopsCleanly(t *testing.T) {
	t.Parallel()
	s, err := startNATS()
	if err != nil {
		t.Fatal(err)
	}
	dir := s.dir
	t.Logf("embedded nats-server ready in %s", s.startup)
	if s.startup <= 0 {
		t.Fatalf("startup = %s, want a measured duration", s.startup)
	}
	if _, err := s.js.AccountInfo(t.Context()); err != nil {
		t.Fatalf("JetStream not enabled on the embedded server: %v", err)
	}
	s.stop()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("store dir %s survived stop: %v", dir, err)
	}
}
