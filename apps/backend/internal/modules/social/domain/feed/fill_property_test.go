package feed_test

import (
	"math/rand/v2"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

type swapOutcome struct {
	id        int
	confirmed bool
	tokens    uint64
	micros    uint64
}

func applyOutcomes(t *testing.T, batch int, outcomes []swapOutcome) feed.Payload {
	t.Helper()
	p := passed()
	for _, o := range outcomes {
		var err error
		if o.confirmed {
			err = p.RecordFill(swapID(o.id), fillOf(o.tokens, o.micros, o.id), batch)
		} else {
			err = p.RecordFailure(swapID(o.id), "failed", batch)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func randomSet(rng *rand.Rand, batch int) []swapOutcome {
	set := make([]swapOutcome, 0, batch)
	for id := range batch {
		if rng.IntN(5) == 0 {
			continue
		}
		set = append(set, swapOutcome{
			id: id, confirmed: rng.IntN(2) == 0, tokens: rng.Uint64N(1_000_000), micros: rng.Uint64N(1_000_000),
		})
	}
	return set
}

func shuffledWithDuplicates(rng *rand.Rand, set []swapOutcome) []swapOutcome {
	order := append([]swapOutcome(nil), set...)
	for range rng.IntN(4) {
		if len(set) > 0 {
			order = append(order, set[rng.IntN(len(set))])
		}
	}
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	return order
}

func TestDeriveFill_dependsOnlyOnTheSetOfSwapOutcomes(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(7, 11))
	for range 300 {
		batch := 1 + rng.IntN(8)
		set := randomSet(rng, batch)
		wantState, wantTokens, wantUSDC, wantAt, err := feed.DeriveFill(applyOutcomes(t, batch, set))
		if err != nil {
			t.Fatal(err)
		}
		for range 5 {
			order := shuffledWithDuplicates(rng, set)
			state, tokens, usdc, at, err := feed.DeriveFill(applyOutcomes(t, batch, order))
			if err != nil || state != wantState || tokens != wantTokens || usdc != wantUSDC || !at.Equal(wantAt) {
				t.Fatalf("batch %d set %+v order %+v: got %s %s %s %s %v, want %s %s %s %s",
					batch, set, order, state, tokens, usdc, at, err, wantState, wantTokens, wantUSDC, wantAt)
			}
		}
	}
}
