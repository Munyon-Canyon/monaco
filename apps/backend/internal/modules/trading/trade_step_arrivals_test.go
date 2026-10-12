package trading_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const arrivalTx = "0xe14c26e97722468e33b999c98c7a747512e1d63be0eb74f724ec4d24abb3c36f"

func (e stepEnv) submittedStep(t *testing.T, ref domain.StepRef, amount uint64) {
	t.Helper()
	if _, err := e.journal.Begin(stepCtx(t), ref, money.MicrosFromUint64(amount)); err != nil {
		t.Fatal(err)
	}
	moved, err := e.journal.Submitted(stepCtx(t), ref, "privy-"+ref.SwapID.String())
	wantMoved(t, "Submitted", moved, err, true)
}

func (e stepEnv) another(t *testing.T) domain.StepRef {
	t.Helper()
	row := e.created(e.ids.NewV7(), usdcMint)
	e.insert(t, row)
	ref := e.ref
	ref.SwapID, ref.CabalID = ids.SwapIDFrom(row.ID), ids.CabalIDFrom(row.CabalID)
	return ref
}

func arrived(amount uint64, tx string) domain.Arrival {
	return domain.Arrival{Amount: money.MicrosFromUint64(amount), TxHash: tx}
}

func TestJournal_succeededRecordsTheArrivalAndItsFee(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.submittedStep(t, e.ref, 1_000_000)

	moved, err := e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(27_493), arrived(972_507, arrivalTx))
	wantMoved(t, "Succeeded", moved, err, true)

	if s := e.step(t); s.Status != domain.StepSucceeded || s.Arrival != arrived(972_507, arrivalTx) {
		t.Fatalf("step = %+v, want succeeded with the 972507 arrival", s)
	}
	got := e.events(t)
	if last := got[len(got)-1]; last.Status != "succeeded" || last.FeeMicros != money.MicrosFromUint64(27_493) {
		t.Fatalf("last event = %+v, want succeeded with fee 27493", last)
	}
}

func TestJournal_anArrivalThatDisagreesWithTheFeeIsRefused(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fee     uint64
		arrival domain.Arrival
	}{
		"fee is the estimate": {fee: 25_930, arrival: arrived(972_507, arrivalTx)},
		"more than was sent":  {fee: 0, arrival: arrived(1_000_001, arrivalTx)},
		"nothing arrived":     {fee: 1_000_000, arrival: arrived(0, arrivalTx)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newStepEnv(t)
			e.submittedStep(t, e.ref, 1_000_000)

			moved, err := e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(tc.fee), tc.arrival)

			if errs.CodeOf(err) != errs.CodeInvalidInput || moved {
				t.Fatalf("Succeeded = %v, %v, want invalid_input", moved, err)
			}
			if s := e.step(t); s.Status != domain.StepSubmitted || s.Arrival.Observed() {
				t.Fatalf("step = %+v, want still submitted with no arrival", s)
			}
		})
	}
}

func TestJournal_arrivedRecordsALateArrivalOnceWithOneEvent(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.submittedStep(t, e.ref, 1_000_000)
	moved, err := e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(25_930), domain.Arrival{})
	wantMoved(t, "Succeeded", moved, err, true)

	first, err := e.journal.Arrived(stepCtx(t), e.ref, arrived(972_507, arrivalTx))
	if err != nil || first.Arrival != arrived(972_507, arrivalTx) {
		t.Fatalf("Arrived = %+v, %v, want the 972507 arrival stored", first.Arrival, err)
	}
	again, err := e.journal.Arrived(stepCtx(t), e.ref, arrived(974_070, "0x01"))
	if err != nil || again.Arrival != first.Arrival {
		t.Fatalf("second Arrived = %+v, %v, want the first arrival kept", again.Arrival, err)
	}

	action := "privy-" + e.ref.SwapID.String()
	wantEvents(t, e.events(t),
		e.amountEvent(domain.StepPending, "", 0),
		e.amountEvent(domain.StepSubmitted, action, 0),
		e.amountEvent(domain.StepSucceeded, action, 25_930),
		e.amountEvent(domain.StepSucceeded, action, 27_493),
	)
}

func (e stepEnv) amountEvent(status domain.StepStatus, action string, fee uint64) busevents.TradeStepChanged {
	ev := e.event(status, action, fee)
	ev.AmountMicros = money.MicrosFromUint64(1_000_000)
	return ev
}

func TestJournal_arrivedLeavesAStepThatHasNotSucceeded(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.submittedStep(t, e.ref, 1_000_000)

	got, err := e.journal.Arrived(stepCtx(t), e.ref, arrived(972_507, arrivalTx))

	if err != nil || got.Arrival.Observed() || e.step(t).Arrival.Observed() {
		t.Fatalf("Arrived on a submitted step = %+v, %v, want no arrival", got.Arrival, err)
	}
}

func TestJournal_arrivedRefusesAMissingStepOrAnEmptyArrival(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	if _, err := e.journal.Arrived(stepCtx(t), e.ref, arrived(972_507, arrivalTx)); err == nil {
		t.Fatal("Arrived on a step that never began succeeded")
	}
	e.submittedStep(t, e.ref, 1_000_000)
	moved, err := e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(25_930), domain.Arrival{})
	wantMoved(t, "Succeeded", moved, err, true)

	_, err = e.journal.Arrived(stepCtx(t), e.ref, arrived(0, arrivalTx))

	if errs.CodeOf(err) != errs.CodeInvalidInput || e.step(t).Arrival.Observed() {
		t.Fatalf("Arrived of nothing err = %v, want invalid_input and no arrival", err)
	}
}

func TestJournal_twoBridgeOutsNeverClaimTheSameArrivalTx(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	other := e.another(t)
	e.submittedStep(t, e.ref, 1_000_000)
	e.submittedStep(t, other, 1_000_000)
	moved, err := e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(27_493), arrived(972_507, arrivalTx))
	wantMoved(t, "first claim", moved, err, true)

	moved, err = e.journal.Succeeded(stepCtx(t), other, money.MicrosFromUint64(27_493), arrived(972_507, arrivalTx))

	if err == nil || moved {
		t.Fatalf("second claim = %v, %v, want refused", moved, err)
	}
	txs, err := e.journal.ArrivalTxs(stepCtx(t), e.ref.Network)
	if err != nil || len(txs) != 1 || txs[0] != arrivalTx {
		t.Fatalf("ArrivalTxs = %v, %v, want only the first claim", txs, err)
	}
}

func TestJournal_aRecordedFeePlusTheArrivalIsWhatWasSent(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	rapid.Check(t, func(rt *rapid.T) {
		sent := rapid.Uint64Range(1, 1<<53).Draw(rt, "sent")
		got := rapid.Uint64Range(1, sent).Draw(rt, "arrived")
		ref := e.another(t)
		e.submittedStep(t, ref, sent)
		fee := money.MicrosFromUint64(sent - got)

		moved, err := e.journal.Succeeded(stepCtx(t), ref, fee, arrived(got, ref.SwapID.String()))
		if err != nil || !moved {
			rt.Fatalf("Succeeded = %v, %v", moved, err)
		}
		s, _, err := e.journal.Step(stepCtx(t), ref.SwapID, ref.Step)
		if err != nil {
			rt.Fatal(err)
		}
		total, err := s.Arrival.Amount.Add(fee)
		if err != nil || total != s.Amount {
			rt.Fatalf("arrived %s + fee %s = %s, want sent %s", s.Arrival.Amount, fee, total, s.Amount)
		}
	})
}
