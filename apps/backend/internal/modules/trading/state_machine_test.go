package trading_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
)

func events() []domain.Event {
	evs := make([]domain.Event, 0, 2+len(domain.FailureCodes()))
	evs = append(evs, domain.Submit(), domain.Confirm())
	for _, code := range domain.FailureCodes() {
		evs = append(evs, domain.Fail(code))
	}
	return evs
}

func eventName(ev domain.Event) string {
	if ev.Kind() == domain.KindFail {
		return fmt.Sprintf("fail(%s)", ev.Failure())
	}
	return string(ev.Kind())
}

func wantNext() map[domain.Status]map[string]domain.Status {
	return map[domain.Status]map[string]domain.Status{
		domain.StatusCreated: {
			"submit":                domain.StatusSubmitted,
			"fail(never_submitted)": domain.StatusFailed,
		},
		domain.StatusSubmitted: {
			"confirm":                 domain.StatusConfirmed,
			"fail(blockhash_expired)": domain.StatusFailed,
			"fail(jupiter_failed)":    domain.StatusFailed,
			"fail(force_resolved)":    domain.StatusFailed,
			"fail(source_cancelled)":  domain.StatusFailed,
		},
		domain.StatusConfirmed: {},
		domain.StatusFailed:    {},
	}
}

func TestNext_everyStatusAndEventPair(t *testing.T) {
	t.Parallel()
	want := wantNext()
	for _, from := range domain.Statuses() {
		for _, ev := range events() {
			to, err := domain.Next(from, ev)
			allowed, ok := want[from][eventName(ev)]
			switch {
			case ok && (err != nil || to != allowed):
				t.Errorf("Next(%s, %s) = %s, %v, want %s", from, eventName(ev), to, err, allowed)
			case !ok && (errs.CodeOf(err) != errs.CodeVersionConflict || to != from):
				t.Errorf("Next(%s, %s) = %s, %v, want %s and version_conflict", from, eventName(ev), to, err, from)
			}
		}
	}
}

func TestNext_rejectsAFailureCodeItDoesNotKnow(t *testing.T) {
	t.Parallel()
	for _, from := range []domain.Status{domain.StatusCreated, domain.StatusSubmitted} {
		if _, err := domain.Next(from, domain.Fail("gremlins")); errs.CodeOf(err) != errs.CodeVersionConflict {
			t.Errorf("Next(%s, fail(gremlins)) err = %v, want version_conflict", from, err)
		}
	}
}

func (d swapDB) at(t *testing.T, status domain.Status) uuid.UUID {
	t.Helper()
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	sig := row.ID.String()
	switch status {
	case domain.StatusCreated:
	case domain.StatusSubmitted:
		d.submit(t, row.ID, sig, sig)
	case domain.StatusConfirmed:
		d.submit(t, row.ID, sig, sig)
		d.confirm(t, row.ID)
	case domain.StatusFailed:
		d.fail(t, row.ID, string(domain.FailureNeverSubmitted))
	}
	return row.ID
}

func (d swapDB) apply(t *testing.T, id uuid.UUID, ev domain.Event) int64 {
	t.Helper()
	switch ev.Kind() {
	case domain.KindSubmit:
		return d.submit(t, id, "req-"+id.String(), "sig-"+id.String())
	case domain.KindConfirm:
		return d.confirm(t, id)
	case domain.KindFail:
		return d.fail(t, id, string(ev.Failure()))
	}
	t.Fatalf("unknown event %s", ev.Kind())
	return 0
}

func TestNext_matchesTheGuardedUpdatesInTheSwapsQueries(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	for _, from := range domain.Statuses() {
		for _, ev := range events() {
			id := d.at(t, from)
			to, err := domain.Next(from, ev)
			rows := d.apply(t, id, ev)
			if (err == nil) != (rows == 1) {
				t.Errorf(
					"%s + %s: Next err = %v but the guarded update changed %d rows",
					from,
					eventName(ev),
					err,
					rows,
				)
			}
			if got := domain.Status(d.status(t, id)); err == nil && got != to {
				t.Errorf("%s + %s: row status = %s, Next = %s", from, eventName(ev), got, to)
			}
		}
	}
}

func checkParse[T ~string](t *testing.T, name string, parse func(string) (T, error), known []T) {
	t.Helper()
	for _, v := range known {
		if got, err := parse(string(v)); err != nil || got != v {
			t.Errorf("%s(%s) = %s, %v", name, v, got, err)
		}
	}
	for _, raw := range []string{"", "Created", "unplanned_kind", "hold"} {
		if got, err := parse(raw); errs.CodeOf(err) != errs.CodeDecodeFailed || got != "" {
			t.Errorf("%s(%q) = %q, %v, want decode_failed", name, raw, got, err)
		}
	}
}

func TestParse_acceptsEachKnownValueAndRejectsTheRest(t *testing.T) {
	t.Parallel()
	checkParse(t, "ParseStatus", domain.ParseStatus, domain.Statuses())
	checkParse(t, "ParseFailureCode", domain.ParseFailureCode, domain.FailureCodes())
	checkParse(t, "ParseSourceKind", domain.ParseSourceKind, domain.SourceKinds())
	checkParse(t, "ParseAction", domain.ParseAction, domain.Actions())
}
