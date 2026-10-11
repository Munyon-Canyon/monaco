package events_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestMultichainEventsNameTheirAggregate(t *testing.T) {
	t.Parallel()
	forward, swap := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	for _, tc := range []struct {
		event     events.Event
		typ       events.Type
		aggregate string
		id        uuid.UUID
	}{
		{events.DepositEVMForwarded{ForwardID: forward}, events.TypeDepositEVMForwarded, "evm_deposit_forward", forward},
		{events.TradeRouted{SwapID: swap}, events.TypeTradeRouted, "swap", swap},
		{events.TradeStepChanged{SwapID: swap}, events.TypeTradeStepChanged, "swap", swap},
	} {
		if tc.event.Type() != tc.typ || tc.event.AggregateType() != tc.aggregate || tc.event.AggregateID() != tc.id {
			t.Errorf("%T = %s %s %s, want %s %s %s", tc.event, tc.event.Type(), tc.event.AggregateType(),
				tc.event.AggregateID(), tc.typ, tc.aggregate, tc.id)
		}
	}
}
