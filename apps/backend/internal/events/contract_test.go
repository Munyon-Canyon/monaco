package events_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	goldenDir   = "testdata/golden"
	usdcMint    = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	aaplxMint   = chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	txSignature = chain.Signature(
		"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
	)
)

func golden() fs.FS { return os.DirFS(goldenDir) }

func fixtures(t *testing.T) map[events.Type]any {
	t.Helper()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8057")
	if err != nil {
		t.Fatal(err)
	}
	user, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8058")
	if err != nil {
		t.Fatal(err)
	}
	sampled := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	cabal := newCabalFixtures(t)
	return map[events.Type]any{
		events.TypeSystemPinged: events.SystemPinged{V: 1, PingID: id, UserID: user, Note: "reference flow"},
		events.TypeDepositCredited: events.DepositCredited{
			V:             1,
			DepositID:     id,
			UserID:        user,
			WalletAddress: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P",
			AmountMicros:  money.MicrosFromUint64(25_000_000),
			TxSignature:   txSignature,
			Slot:          123456,
			BlockTime:     &sampled,
		},
		events.TypeAssetPriceMoved: events.AssetPriceMoved{
			V: 1, AssetID: id, Symbol: "AAPLx", AssetName: "Apple", ThresholdBps: 1000, ChangeBps: 1234,
			MarkMicros: money.MicrosFromUint64(220_000_000), PrevCloseMicros: money.MicrosFromUint64(200_000_000),
			TradingDay: "2026-03-02", ObservedAt: time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC),
		},
		events.TypePriceTick: events.PriceTick{V: 1, AsOf: sampled, Prices: []events.TickPrice{{
			Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", AssetID: id,
			PriceMicros: money.MicrosFromUint64(254_371_234), ObservedAt: sampled.Add(-2 * time.Minute),
		}}},
		events.TypeCabalCreated:         cabal.created,
		events.TypeCabalMemberJoined:    cabal.memberJoined,
		events.TypeCabalAccessRequested: cabal.accessRequested,
		events.TypeCabalAccessDecided:   cabal.accessDecided,
		events.TypeCabalMemberLeft:      cabal.memberLeft,
		events.TypeCabalUpdated:         cabal.updated,
		events.TypeTradeBlocked: events.TradeBlocked{
			V: 1, CabalID: user, Source: events.TradeSource{Kind: "proposal", ID: id}, SourceBatchSize: 1,
			Action: "buy", Symbol: "AAPLx", Code: errs.CodeSlippageExceeded, Have: 104_000_000, Need: 104_475_000,
		},
		events.TypeTradeSubmitted: events.TradeSubmitted{
			V: 1, SwapID: id, CabalID: user, Source: events.TradeSource{Kind: "proposal", ID: user}, SourceBatchSize: 1,
			Action: "buy", Symbol: "AAPLx", InMint: usdcMint, OutMint: aaplxMint, InAmount: 25_000_000,
			TxSignature: txSignature,
		},
		events.TypeTradeConfirmed: events.TradeConfirmed{
			V: 1, SwapID: id, CabalID: user, Source: events.TradeSource{Kind: "cashout", ID: user}, SourceBatchSize: 2,
			Action: "sell", Symbol: "AAPLx", InMint: aaplxMint, InAmount: 105_000_000, OutMint: usdcMint,
			OutAmount: 24_950_000, USDCMicros: money.MicrosFromUint64(24_950_000),
			FeeMicros: money.MicrosFromUint64(5_000), TxSignature: txSignature,
			ConfirmedAt: time.Date(2026, 3, 1, 12, 0, 30, 0, time.UTC),
		},
		events.TypeTradeFailed: events.TradeFailed{
			V: 1, SwapID: id, CabalID: user, Source: events.TradeSource{Kind: "proposal", ID: user}, SourceBatchSize: 1,
			Action: "buy", Symbol: "AAPLx", InMint: usdcMint, InAmount: 25_000_000, FailureCode: "jupiter_failed",
			JupiterCode: "-1004",
		},
	}
}

func proposalFixtures(t *testing.T) map[events.Type]any {
	t.Helper()
	g := testkit.NewIDs(528)
	proposal, cabal, proposer, swap := g.NewV7(), g.NewV7(), g.NewV7(), g.NewV7()
	return map[events.Type]any{
		events.TypeProposalCreated: events.ProposalCreated{
			V: 1, ProposalID: proposal, CabalID: cabal, ProposerID: proposer, Kind: "buy", Symbol: "AAPLx",
			Mint: aaplxMint, USDCMicros: money.MicrosFromUint64(25_000_000), QuoteOutAmount: 105_000_000,
			ExpiresAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), VoterCount: 3,
		},
		events.TypeProposalPassed: events.ProposalPassed{
			V: 1, ProposalID: proposal, CabalID: cabal, Kind: "sell", Symbol: "AAPLx", Mint: aaplxMint,
			TokenAmount: 300_000_000, QuoteOutAmount: 71_250_000, ProposerID: proposer,
		},
		events.TypeProposalFailed:  events.ProposalFailed{V: 1, ProposalID: proposal, CabalID: cabal},
		events.TypeProposalExpired: events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal},
		events.TypeProposalWithdrawn: events.ProposalWithdrawn{
			V:          1,
			ProposalID: proposal,
			CabalID:    cabal,
			ProposerID: proposer,
		},
		events.TypeProposalVoided: events.ProposalVoided{
			V: 1, ProposalID: proposal, CabalID: cabal, ActorType: "admin", Reason: "Duplicate of another proposal.",
		},
		events.TypeProposalExecuted: events.ProposalExecuted{V: 1, ProposalID: proposal, CabalID: cabal, SwapID: swap},
		events.TypeProposalExecutionBlocked: events.ProposalExecutionBlocked{
			V: 1, ProposalID: proposal, CabalID: cabal, Code: errs.CodePotExceeded,
		},
	}
}

func userFixtures(t *testing.T) map[events.Type]any {
	t.Helper()
	g := testkit.NewIDs(572)
	user := g.NewV7()
	return map[events.Type]any{
		events.TypeUserCreated: events.UserCreated{
			V: 1, UserID: user, LoginProvider: "sms", CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		},
		events.TypeUserAuthStateChanged: events.UserAuthStateChanged{
			V: 1, UserID: user, From: "ONBOARDING_COMPLETED", To: "AWAITING_PHONE", Cause: "unlink",
			At: time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC),
		},
		events.TypeUserProfileUpdated: events.UserProfileUpdated{
			V: 1, UserID: user, Fields: []string{"handle", "photo"}, Handle: "kaicenat", DisplayName: "Kai Cenat",
			PhotoURL: "https://cdn.example.com/photos/kai.jpg",
		},
		events.TypeUserDeleted: events.UserDeleted{V: 1, UserID: user, At: time.Date(2026, 3, 3, 8, 0, 0, 0, time.UTC)},
		events.TypeUserNudgeDue: events.UserNudgeDue{
			V: 1, UserID: user, Kind: "add_phone", NudgeNumber: 1, At: time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC),
		},
	}
}

func followFixtures(t *testing.T) map[events.Type]any {
	t.Helper()
	g := testkit.NewIDs(569)
	follow, follower, followee := g.NewV7(), g.NewV7(), g.NewV7()
	return map[events.Type]any{
		events.TypeFollowCreated: events.FollowCreated{
			V: 1, FollowID: follow, FollowerID: follower, FolloweeID: followee, Source: "profile",
			CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		},
		events.TypeFollowRemoved: events.FollowRemoved{
			V: 1, FollowID: follow, FollowerID: follower, FolloweeID: followee,
			RemovedAt: time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC),
		},
	}
}

func pauseFixtures(t *testing.T) map[events.Type]any {
	t.Helper()
	g := testkit.NewIDs(1683)
	pause, cabal := g.NewV7(), g.NewV7()
	return map[events.Type]any{
		events.TypeCabalPaused: events.CabalPaused{
			V: 1, PauseID: pause, CabalID: &cabal, Reason: "external_deposit", Scope: "cabal",
		},
		events.TypeCabalResumed: events.CabalResumed{V: 1, CabalID: &cabal, Scope: "cabal"},
	}
}

func goldenName(t events.Type, v int) string { return fmt.Sprintf("%s.v%d.json", t, v) }

func TestGoldenPayloads(t *testing.T) {
	t.Parallel()
	fx := fixtures(t)
	maps.Copy(fx, proposalFixtures(t))
	maps.Copy(fx, userFixtures(t))
	maps.Copy(fx, followFixtures(t))
	maps.Copy(fx, pauseFixtures(t))
	for _, entry := range events.Catalog() {
		ev, ok := fx[entry.Type]
		if !ok {
			t.Errorf("%s has no fixture", entry.Type)
			continue
		}
		got, err := json.MarshalIndent(ev, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		name := goldenName(entry.Type, entry.Version)
		if *update {
			if err := os.WriteFile(filepath.Join(goldenDir, name), got, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := fs.ReadFile(golden(), name)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./internal/events -update)", entry.Type, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s payload drifted from %s\n got: %s\nwant: %s", entry.Type, name, got, want)
		}
	}
}

func TestEveryGoldenFileDecodesAsARegisteredVersion(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(golden(), "*.json")
	if err != nil {
		t.Fatal(err)
	}
	current := map[events.Type]int{}
	core := map[events.Type]bool{}
	for _, entry := range events.Catalog() {
		current[entry.Type] = entry.Version
		core[entry.Type] = entry.Core
	}
	name := regexp.MustCompile(`^(.+)\.v([0-9]+)\.json$`)
	for _, path := range files {
		m := name.FindStringSubmatch(path)
		if m == nil {
			t.Errorf("%s is not named <type>.v<n>.json", path)
			continue
		}
		typ := events.Type(m[1])
		v, _ := strconv.Atoi(m[2])
		if _, ok := current[typ]; !ok {
			t.Errorf("%s has no registered type", path)
			continue
		}
		if core[typ] {
			delete(current, typ)
			continue
		}
		payload, err := fs.ReadFile(golden(), path)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(typ, v, payload)
		if err != nil || ev.Type() != typ {
			t.Errorf("%s: Decode = %v, %v", path, ev, err)
		}
		delete(current, typ)
	}
	for typ := range current {
		t.Errorf("%s has no golden file", typ)
	}
}

func TestCorePayloadsRoundTripThroughTheirGoType(t *testing.T) {
	t.Parallel()
	fx := fixtures(t)
	for _, entry := range events.Catalog() {
		if !entry.Core {
			continue
		}
		want, err := fs.ReadFile(golden(), goldenName(entry.Type, entry.Version))
		if err != nil {
			t.Fatal(err)
		}
		decoded := reflect.New(reflect.TypeOf(fx[entry.Type]))
		if err := json.Unmarshal(want, decoded.Interface()); err != nil {
			t.Fatalf("%s: %v", entry.Type, err)
		}
		got, err := json.MarshalIndent(decoded.Elem().Interface(), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if got = append(got, '\n'); !bytes.Equal(got, want) {
			t.Errorf("%s did not survive a decode\n got: %s\nwant: %s", entry.Type, got, want)
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, entry := range events.Catalog() {
		payload, err := fs.ReadFile(golden(), goldenName(entry.Type, entry.Version))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(entry.Type), entry.Version, payload)
	}
	f.Fuzz(func(t *testing.T, typ string, v int, payload []byte) {
		ev, err := events.Decode(events.Type(typ), v, payload)
		if err != nil {
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.CodeDecodeFailed || ev != nil {
				t.Fatalf("Decode(%q, %d) = %v, %v; want nil and decode_failed", typ, v, ev, err)
			}
			return
		}
		if ev.Type() != events.Type(typ) {
			t.Fatalf("Decode(%q, %d) returned %s", typ, v, ev.Type())
		}
	})
}
