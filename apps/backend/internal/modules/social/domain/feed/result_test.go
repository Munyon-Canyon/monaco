package feed_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func TestResultShape_namesWhoHeadsEachResult(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name      string
		action    feed.Action
		outcome   feed.Outcome
		assetKind feed.AssetKind
		want      feed.Shape
	}{
		{"passed listed buy", feed.ActionBuy, feed.OutcomePassed, feed.AssetKindEquity, feed.ShapeCabal},
		{"passed sale", feed.ActionSell, feed.OutcomePassed, feed.AssetKindEquity, feed.ShapeMember},
		{"passed pre-ipo buy", feed.ActionBuy, feed.OutcomePassed, feed.AssetKindPreIPO, feed.ShapeMember},
		{"passed buy of an unknown kind", feed.ActionBuy, feed.OutcomePassed, "", feed.ShapeCabal},
		{"failed buy", feed.ActionBuy, feed.OutcomeFailed, feed.AssetKindEquity, feed.ShapeCabal},
		{"failed sale", feed.ActionSell, feed.OutcomeFailed, feed.AssetKindEquity, feed.ShapeCabal},
		{"failed pre-ipo buy", feed.ActionBuy, feed.OutcomeFailed, feed.AssetKindPreIPO, feed.ShapeCabal},
	} {
		if got := feed.ResultShape(c.action, c.outcome, c.assetKind); got != c.want {
			t.Errorf("%s: ResultShape = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPayload_roundTripsTheCloseFactsAndKeepsReservedFillKeys(t *testing.T) {
	t.Parallel()
	want := feed.Payload{
		Outcome: feed.OutcomePassed, Yes: 4, No: 1, Voters: 6, Needed: 4, Shape: feed.ShapeCabal,
		CallerID: uuid.NewSHA1(uuid.Nil, []byte("caller")), CallerName: "Christelle",
		AssetKind: feed.AssetKindEquity, Fills: json.RawMessage(`[{"n":1}]`), Failures: json.RawMessage(`["x"]`),
		ExpectedFills: 2, FillState: "partial",
	}
	got, err := feed.ParsePayload(want.JSON())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePayload = %+v, %v, want %+v", got, err, want)
	}
}
