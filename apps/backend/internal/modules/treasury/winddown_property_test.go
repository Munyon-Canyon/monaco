package treasury_test

import (
	"testing"

	"pgregory.net/rapid"
)

func TestWindDownProperty_PaysEachMemberTheirSliceAndBalancesTheLedger(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	rapid.Check(t, func(rt *rapid.T) {
		deposits, pot := drawDeposits(rt)
		r := newWindDownRigOn(t, f, deposits...)
		if err := r.start(); err != nil {
			rt.Fatalf("start = %v", err)
		}
		if held, jobs := r.held(t), r.jobs(t); held != 0 || jobs != int64(len(deposits)) {
			rt.Fatalf("%d jobs and %d shares held across %d members, want a job each and nothing held",
				jobs, held, len(deposits))
		}
		units, payouts := issuedJobs(t, rt, r)
		assertSlices(rt, pot, units, payouts)
		if drift := r.f.drift(t); len(drift) != 0 {
			rt.Fatalf("ledger drift = %v", drift)
		}
	})
}

func drawDeposits(rt *rapid.T) ([]deposit, int64) {
	deposits := make([]deposit, rapid.IntRange(1, 5).Draw(rt, "members"))
	var pot int64
	for i := range deposits {
		deposits[i] = deposit{
			micros: rapid.Int64Range(1, 500_000_000).Draw(rt, "micros"),
			shares: rapid.Int64Range(1, 1_000_000).Draw(rt, "shares"),
		}
		pot += deposits[i].micros
	}
	return deposits, pot
}

func issuedJobs(t *testing.T, rt *rapid.T, r windDownRig) (units, payouts []int64) {
	t.Helper()
	rows, err := r.f.pool.Query(t.Context(),
		`SELECT share_units::bigint, payout_micros::bigint FROM cash_out_jobs WHERE cabal_id = $1 ORDER BY id`,
		r.cabal.UUID())
	if err != nil {
		rt.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var u, p int64
		if err := rows.Scan(&u, &p); err != nil {
			rt.Fatal(err)
		}
		units, payouts = append(units, u), append(payouts, p)
	}
	if err := rows.Err(); err != nil {
		rt.Fatal(err)
	}
	return units, payouts
}

func assertSlices(rt *rapid.T, pot int64, units, payouts []int64) {
	var total int64
	for _, u := range units {
		total += u
	}
	left := pot
	for i, u := range units {
		want := left * u / total
		if payouts[i] != want {
			rt.Fatalf("payout %d = %d, want its slice %d of %d (%d of %d units)", i, payouts[i], want, left, u, total)
		}
		left, total = left-want, total-u
	}
	if left != 0 {
		rt.Fatalf("%d of the %d pot left unpaid", left, pot)
	}
}
