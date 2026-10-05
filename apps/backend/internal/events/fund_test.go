package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFundEvents_areKeyedByTheirTransfer(t *testing.T) {
	t.Parallel()
	transfer := testkit.NewIDs(1).NewV7()
	for _, tt := range []struct {
		event events.Event
		want  events.Type
	}{
		{events.FundSubmitted{TransferID: transfer}, events.TypeFundSubmitted},
		{events.Funded{TransferID: transfer}, events.TypeFunded},
		{events.FundFailed{TransferID: transfer}, events.TypeFundFailed},
	} {
		if tt.event.Type() != tt.want || tt.event.AggregateType() != "fund_transfer" ||
			tt.event.AggregateID() != transfer {
			t.Errorf("%T = %s/%s/%s, want %s/fund_transfer/%s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.want, transfer)
		}
	}
}
