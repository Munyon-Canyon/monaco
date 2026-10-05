package trading_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func stubBus(t *testing.T) *engineBus {
	t.Helper()
	cfg := testkit.Config()
	cfg.Trade.Engine = config.TradeEngineStub
	return newEngineBusWith(t, cfg)
}

func TestTradeEngineStub_ClaimsTheDeliveryOnceAndTradesNothing(t *testing.T) {
	t.Parallel()
	b := stubBus(t)
	cmd := b.buy()
	msg := b.message(t, cmd, "")

	b.dispatch(t, msg)
	got := b.count(t, `SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id::text = $2
		AND code = 'stubbed'`, engineHandler, msg.id.String())
	if msg.verdict != "ack" || got != 1 {
		t.Fatalf("verdict %q, %d stubbed deliveries; want an ack and one", msg.verdict, got)
	}
	if n := len(b.swapsOf(t, cmd.ProposalID)); n != 0 {
		t.Fatalf("%d swaps; the stub must write none", n)
	}
	if n := b.count(t, `SELECT count(*) FROM events WHERE type LIKE 'trade.%'`); n != 0 {
		t.Fatalf("%d trade events; the stub must append none", n)
	}
}

func TestTradeEngineStub_RedeliveryWritesNothing(t *testing.T) {
	t.Parallel()
	b := stubBus(t)
	msg := b.message(t, b.buy(), "")

	b.dispatch(t, msg)
	events := b.count(t, `SELECT count(*) FROM events`)
	again := b.redeliver(t, msg)
	if again.verdict != "ack" || b.count(t, `SELECT count(*) FROM events`) != events ||
		b.count(t, `SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id::text = $2`,
			engineHandler, msg.id.String()) != 1 {
		t.Fatalf("verdict %q; a redelivery must ack and write nothing", again.verdict)
	}
}

func TestTradeEngineStub_IsRefusedOutsideLocalDev(t *testing.T) {
	t.Parallel()
	const local = "postgres://monaco@localhost:54322/monaco"
	tests := []struct {
		name string
		env  string
		db   string
		ok   bool
	}{
		{"local dev", "local", local, true},
		{"staging", "staging", local, false},
		{"test env", "test", local, false},
		{"remote database", "local", "postgres://monaco@db.example.com:5432/monaco", false},
		{"other database", "local", "postgres://monaco@localhost:54322/other", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load([]string{
				"MONACO_ENV=" + tc.env, "DATABASE_URL=" + tc.db, "NATS_URL=nats://localhost:4222",
				"APNS_KEY_P8=p8", "APNS_KEY_ID=id", "APNS_TEAM_ID=team", "TRADE_ENGINE=stub",
			})
			if tc.ok != (err == nil) || (!tc.ok && !strings.Contains(err.Error(), "TRADE_ENGINE")) {
				t.Fatalf("Load err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}
