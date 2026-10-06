package notify_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const pollerActor = "system:poller.funding.deposits"

func TestNotify_DepositCredited_Depositor(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	depositor, bystander := r.user(t, "active"), r.user(t, "active")
	r.device(t, depositor, token('a'))
	r.device(t, bystander, token('b'))
	e := goldenEvent(t, events.TypeDepositCredited).(events.DepositCredited)
	e.UserID = depositor.UUID()
	d := r.emit(t, pollerActor, e)

	wantVerdict(t, handleKinds(t, r, r.sender, d, e, app.DepositCredited{}), "", errs.VerdictAck)

	want := apns.Push{
		UserID: depositor, Token: token('a'), Environment: apns.Sandbox, CollapseID: "deposit-" + e.DepositID.String(),
		Title: "Deposit received", Body: "$25.00 is in your account balance.",
		Data: map[string]string{"kind": "deposit_credited"},
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{depositor: "delivered"})
	r.wantSentEvents(t, d, 1)
	r.wantRecorded(t, d, 1)
}

func TestNotify_DepositCredited_BodyShowsTruncatedDollars(t *testing.T) {
	t.Parallel()
	for micros, want := range map[uint64]string{
		25_000_000:     "$25.00 is in your account balance.",
		27_500_000:     "$27.50 is in your account balance.",
		25_009_999:     "$25.00 is in your account balance.",
		10_000:         "$0.01 is in your account balance.",
		9_999:          "$0.00 is in your account balance.",
		1_234_560_000:  "$1234.56 is in your account balance.",
		math.MaxUint64: "$18446744073709.55 is in your account balance.",
	} {
		e := events.DepositCredited{V: 1, AmountMicros: money.MicrosFromUint64(micros)}
		msg, err := app.DepositCredited{}.Render(t.Context(), e, ids.UserID{})
		if err != nil || msg.Body != want {
			t.Errorf("%d micros: body %q, %v, want %q", micros, msg.Body, err, want)
		}
	}
}
