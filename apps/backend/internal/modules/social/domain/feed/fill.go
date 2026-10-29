package feed

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	FillPending = "pending"
	FillFilled  = "filled"
	FillPartial = "partial"
	FillFailed  = "failed"
)

type Fill struct {
	TokenAmount money.BaseUnits
	USDCMicros  money.Micros
	FilledAt    time.Time
}

type fillWire struct {
	TokenAmount   string       `json:"token_amount"`
	TokenDecimals uint8        `json:"token_decimals"`
	USDCMicros    money.Micros `json:"usdc_micros"`
	FilledAt      time.Time    `json:"filled_at"`
}

func (f Fill) MarshalJSON() ([]byte, error) {
	raw, _ := json.Marshal(fillWire{
		TokenAmount: f.TokenAmount.String(), TokenDecimals: f.TokenAmount.Decimals(),
		USDCMicros: f.USDCMicros, FilledAt: f.FilledAt,
	})
	return raw, nil
}

func (f *Fill) UnmarshalJSON(raw []byte) error {
	var w fillWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "feed.Fill.UnmarshalJSON")
	}
	amount, err := strconv.ParseUint(w.TokenAmount, 10, 64)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "feed.Fill.UnmarshalJSON", slog.String("raw", w.TokenAmount))
	}
	*f = Fill{TokenAmount: money.NewBaseUnits(amount, w.TokenDecimals), USDCMicros: w.USDCMicros, FilledAt: w.FilledAt}
	return nil
}

func (p *Payload) RecordFill(swapID uuid.UUID, f Fill, batch int) error {
	id := swapID.String()
	if p.seen(id) {
		return nil
	}
	if err := p.expect(batch); err != nil {
		return err
	}
	if p.Fills == nil {
		p.Fills = map[string]Fill{}
	}
	p.Fills[id] = f
	return nil
}

func (p *Payload) RecordFailure(swapID uuid.UUID, code string, batch int) error {
	id := swapID.String()
	if p.seen(id) {
		return nil
	}
	if err := p.expect(batch); err != nil {
		return err
	}
	if p.Failures == nil {
		p.Failures = map[string]string{}
	}
	p.Failures[id] = code
	return nil
}

func (p *Payload) seen(id string) bool {
	_, filled := p.Fills[id]
	_, failed := p.Failures[id]
	return filled || failed
}

func (p *Payload) expect(batch int) error {
	if len(p.Fills)+len(p.Failures) > 0 && p.ExpectedFills != batch {
		return errs.New(errs.CodeFeedFillBatchMismatch, "feed.Payload.expect",
			slog.Int("expected", p.ExpectedFills), slog.Int("got", batch))
	}
	p.ExpectedFills = batch
	return nil
}

func sumFills(fills map[string]Fill) (tokens money.BaseUnits, usdc money.Micros, lastAt time.Time, err error) {
	first := true
	for _, f := range fills {
		if first {
			tokens, first = f.TokenAmount, false
		} else if tokens, err = tokens.Add(f.TokenAmount); err != nil {
			return money.BaseUnits{}, money.Micros{}, time.Time{}, err
		}
		if usdc, err = usdc.Add(f.USDCMicros); err != nil {
			return money.BaseUnits{}, money.Micros{}, time.Time{}, err
		}
		if f.FilledAt.After(lastAt) {
			lastAt = f.FilledAt
		}
	}
	return tokens, usdc, lastAt, nil
}

func DeriveFill(p Payload) (state string, tokens money.BaseUnits, usdc money.Micros, lastAt time.Time, err error) {
	if p.Outcome != OutcomePassed {
		return "", tokens, usdc, lastAt, nil
	}
	if tokens, usdc, lastAt, err = sumFills(p.Fills); err != nil {
		return "", money.BaseUnits{}, money.Micros{}, time.Time{}, err
	}
	failed := 0
	for id := range p.Failures {
		if _, ok := p.Fills[id]; !ok {
			failed++
		}
	}
	reported := len(p.Fills) + failed
	expected := p.ExpectedFills
	if expected == 0 {
		expected = reported
	}
	switch {
	case reported == 0 || reported < expected:
		state = FillPending
	case failed == 0:
		state = FillFilled
	case len(p.Fills) == 0:
		state = FillFailed
	default:
		state = FillPartial
	}
	return state, tokens, usdc, lastAt, nil
}
