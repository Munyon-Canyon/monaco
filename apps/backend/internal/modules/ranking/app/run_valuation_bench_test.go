package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const membersPerCabal = 10

func benchPorts(b *testing.B, cabals int) (*countingPorts, chain.SolanaAddress) {
	b.Helper()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		b.Fatal(err)
	}
	at := valuationTime()
	f := &countingPorts{
		members: map[ids.CabalID][]cabal.MemberView{}, userRows: map[ids.UserID]identity.UserCard{},
	}
	for range cabals {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		f.cabals = append(f.cabals, cabal.View{ID: id, CreatedAt: at.Add(-30 * 24 * time.Hour)})
		f.positionRows = append(f.positionRows, treasury.CabalPositions{
			CabalID:     id,
			Holdings:    []treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(membersPerCabal*1_000_000, 6)}},
			TotalShares: money.SharesUnitsFromUint64(membersPerCabal),
		})
		var bucket int32
		for _, rng := range rangedRanges() {
			t0, _ := rng.Start(at)
			f.snapshotRows = append(f.snapshotRows, sqlc.SnapshotsAtRow{
				Bucket: bucket, CabalID: id.UUID(), At: t0.Add(-time.Minute),
				ValueMicros: membersPerCabal * 1_000_000, TotalShares: membersPerCabal,
			})
			bucket++
		}
		for range membersPerCabal {
			user := ids.UserIDFrom(ids.Real{}.NewV7())
			f.members[id] = append(f.members[id], cabal.MemberView{UserID: user})
			f.userRows[user] = identity.UserCard{ID: user, CreatedAt: at.Add(-time.Hour)}
			f.stakeRows = append(f.stakeRows, treasury.MemberStake{
				CabalID: id, UserID: user, ShareUnits: money.SharesUnitsFromUint64(1),
				NetContributedMicros: money.SignedMicrosFromInt64(1_000_000),
			})
			f.flowRows = append(f.flowRows, treasury.MemberFlow{
				CabalID: id, UserID: user, Amount: money.SignedMicrosFromInt64(100_000), At: at.Add(-30 * time.Minute),
			})
		}
	}
	return f, usdc
}

func BenchmarkRunValuation_500Cabals_5000Members(b *testing.B) {
	f, usdc := benchPorts(b, 500)
	run := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f}, usdc,
	)
	b.ResetTimer()
	for b.Loop() {
		got, err := run.Run(b.Context(), valuationTime())
		if err != nil || len(got.Cabals) != 500 {
			b.Fatalf("Run() = %d cabals, %v", len(got.Cabals), err)
		}
	}
}
