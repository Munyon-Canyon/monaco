package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalmod "github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	trading "github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	devSeedUsage   = "usage: monacoctl dev seed-scenario <name> [--actor A=<user-uuid> ...] [--json]"
	devSeedRefused = "monacoctl dev seed-scenario: refused: only the local dev database"
)

type devScenario func(t testkit.SeedT, deps module.Deps, actor func(letter string) ids.UserID) map[string]string

var errBadActor = errors.New("bad --actor")

func devScenarios() map[string]devScenario {
	return map[string]devScenario{
		"cabal-with-confirmed-trade": seedConfirmedTrade,
		"cabal-with-failed-trade":    seedFailedTrade,
		"cabal-with-funded-pot":      seedFundedPot,
		"cabal-with-members":         seedCabalWithMembers,
		"cabal-with-open-proposal":   seedOpenProposal,
		"user-with-balance":          seedUserWithBalance,
	}
}

type openDB func(ctx context.Context, cfg config.DB) (*pgxpool.Pool, error)

func devSeedScenario(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	return devSeedScenarioOn(cfg, db.Open, args, stdout, stderr)
}

func devSeedScenarioOn(cfg config.Config, open openDB, args []string, stdout, stderr io.Writer) int {
	if !devSeedAllowed(cfg) {
		_, _ = fmt.Fprintln(stderr, devSeedRefused)
		return 2
	}
	name, actors, asJSON, ok := parseDevSeedArgs(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, devSeedUsage)
		_, _ = fmt.Fprintf(stderr, "scenarios: %s\n", strings.Join(slices.Sorted(maps.Keys(devScenarios())), ", "))
		return 2
	}
	pool, err := open(context.Background(), cfg.DB)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev seed-scenario: %v\n", err)
		return 1
	}
	defer pool.Close()
	out, err := seedDevScenario(
		module.Deps{Config: cfg, Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}},
		name,
		actors,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev seed-scenario: %v\n", err)
		return 1
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(out)
		return 0
	}
	for _, k := range slices.Sorted(maps.Keys(out)) {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", k, out[k])
	}
	return 0
}

func devSeedAllowed(cfg config.Config) bool { return cfg.LocalDev() }

func parseDevSeedArgs(args []string) (name string, actors map[string]ids.UserID, asJSON, ok bool) {
	if len(args) == 0 || devScenarios()[args[0]] == nil {
		return "", nil, false, false
	}
	actors = map[string]ids.UserID{}
	fs := flag.NewFlagSet("dev seed-scenario", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&asJSON, "json", false, "print the seeded ids as one JSON object")
	fs.Func("actor", "A=<user-uuid>: the existing user that plays actor A", func(v string) error {
		letter, raw, found := strings.Cut(v, "=")
		user, err := ids.ParseUserID(raw)
		if !found || letter == "" || err != nil {
			return fmt.Errorf("%w %q", errBadActor, v)
		}
		actors[letter] = user
		return nil
	})
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return "", nil, false, false
	}
	return args[0], actors, asJSON, true
}

func seedDevScenario(deps module.Deps, name string, actors map[string]ids.UserID) (out map[string]string, err error) {
	err = testkit.SeedErr(context.Background(), func(t testkit.SeedT) { out = runDevScenario(t, deps, name, actors) })
	return out, err
}

func runDevScenario(t testkit.SeedT, deps module.Deps, name string, actors map[string]ids.UserID) map[string]string {
	actor := func(letter string) ids.UserID {
		if _, ok := actors[letter]; !ok {
			actors[letter] = testkit.SeedUser(t, deps.Pool, testkit.UserOpts{WithWallet: true}).ID
		}
		return actors[letter]
	}
	out := devScenarios()[name](t, deps, actor)
	for letter, user := range actors {
		out["actor_"+letter] = user.String()
	}
	return out
}

const confirmedTradeCabal = "01890a5d-ac96-774b-bcce-b302099a8090"

func seedConfirmedTrade(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	owner := actor("A")
	cabal := testkit.NewCabal(t, deps.Pool, testkit.WithCreator(owner))
	testkit.NewLedger(t, deps.Pool).WithFundedMember(owner, cabal.ID, money.MicrosFromUint64(100_000_000))
	raw := devScenarioJSONL(t, "cabal-with-confirmed-trade", map[string]string{confirmedTradeCabal: cabal.ID.String()})
	out := map[string]string{"cabal_id": cabal.ID.String()}
	for _, s := range testkit.SeedJSONL(t, deps.Pool, "cabal-with-confirmed-trade", raw,
		treasury.New(deps).Consumers()...) {
		if trade, ok := s.Event.(events.TradeConfirmed); ok {
			out["swap_id"], out["asset_mint"], out["symbol"] = trade.SwapID.String(), string(
				trade.OutMint,
			), trade.Symbol
		}
	}
	return out
}

func seedFundedPot(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	run := os.Getenv("MONACO_QA_RUN")
	if run == "" {
		run = ids.Real{}.NewV7().String()[24:30]
	}
	a, b := actor("A"), actor("B")
	cabal := testkit.NewCabal(t, deps.Pool, testkit.WithName("QA "+run), testkit.WithCreator(a),
		testkit.WithJoiner(b))
	testkit.NewLedger(t, deps.Pool).
		WithFundedMember(a, cabal.ID, money.MicrosFromUint64(2_000_000)).
		WithFundedMember(b, cabal.ID, money.MicrosFromUint64(1_000_000))
	return map[string]string{"cabal_id": cabal.ID.String(), "cabal_name": "QA " + run}
}

func seedCabalWithMembers(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	cabal := newDevCabal(t, deps, actor)
	raw := devScenarioJSONL(t, "cabal-with-members", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a8070": cabal.ID.String(),
		"01890a5d-ac96-774b-bcce-b302099a8071": actor("A").String(),
		"01890a5d-ac96-774b-bcce-b302099a8072": actor("B").String(),
		"01890a5d-ac96-774b-bcce-b302099a8073": actor("C").String(),
	})
	testkit.SeedJSONL(t, deps.Pool, "cabal-with-members", raw, cabalmod.New(deps).Consumers()...)
	return map[string]string{"cabal_id": cabal.ID.String()}
}

func seedOpenProposal(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	cabal := newDevCabal(t, deps, actor)
	id := insertDevProposal(t, deps, cabal, actor("A"))
	raw := devScenarioJSONL(t, "cabal-with-open-proposal", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a8091": cabal.ID.String(),
		"01890a5d-ac96-774b-bcce-b302099a8092": actor("A").String(),
		"01890a5d-ac96-774b-bcce-b302099a8096": actor("B").String(),
		"01890a5d-ac96-774b-bcce-b302099a8097": actor("C").String(),
		"01890a5d-ac96-774b-bcce-b302099a8099": id.String(),
	})
	testkit.SeedJSONL(t, deps.Pool, "cabal-with-open-proposal", raw, cabalmod.New(deps).Consumers()...)
	return map[string]string{
		"cabal_id": cabal.ID.String(), "proposal_id": id.String(),
		"asset_mint": devProposalMint, "symbol": "AAPLx",
	}
}

func insertDevProposal(t testkit.SeedT, deps module.Deps, cabal testkit.SeededCabal, proposer ids.UserID) uuid.UUID {
	id, now := ids.Real{}.NewV7(), clock.Real{}.Now().UTC()
	params := governance.InsertProposalParams{
		ID: id, CabalID: cabal.ID.UUID(), ProposerID: proposer.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: devProposalMint, UsdcMicros: pgtype.Int8{Int64: devProposalMicros, Valid: true},
		QuoteOutAmount: devProposalQuote, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	}
	for _, m := range cabal.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	_, err := governance.New(deps.Pool).InsertProposal(t.Context(), params)
	failSeed(t, err, "insert proposal")
	return id
}

func seedFailedTrade(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	cabal := newDevCabal(t, deps, actor)
	proposal := insertDevProposal(t, deps, cabal, actor("A"))
	gov, now := governance.New(deps.Pool), clock.Real{}.Now().UTC()
	for _, m := range cabal.Members {
		failSeed(t, gov.UpsertBallot(t.Context(), governance.UpsertBallotParams{
			ProposalID: proposal, VoterID: m.ID.UUID(), Choice: "yes", CastAt: now,
		}), "cast ballot")
	}
	_, err := gov.Transition(t.Context(), governance.TransitionParams{
		ID: proposal, FromStatus: "open", ToStatus: "passed", At: now,
	})
	failSeed(t, err, "pass proposal")
	raw := devScenarioJSONL(t, "cabal-with-failed-trade", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a80c1": cabal.ID.String(),
		"01890a5d-ac96-774b-bcce-b302099a80c2": actor("A").String(),
		"01890a5d-ac96-774b-bcce-b302099a80c6": actor("B").String(),
		"01890a5d-ac96-774b-bcce-b302099a80c7": actor("C").String(),
		"01890a5d-ac96-774b-bcce-b302099a80c9": proposal.String(),
	})
	out := map[string]string{
		"cabal_id": cabal.ID.String(), "proposal_id": proposal.String(),
		"asset_mint": devProposalMint, "symbol": "AAPLx",
	}
	consumers := append(cabalmod.New(deps).Consumers(), treasury.New(deps).Consumers()...)
	for _, s := range testkit.SeedJSONL(t, deps.Pool, "cabal-with-failed-trade", raw, consumers...) {
		if trade, ok := s.Event.(events.TradeSubmitted); ok {
			insertFailedSwap(t, deps, cabal, trade, now)
			out["swap_id"] = trade.SwapID.String()
		}
	}
	return out
}

func insertFailedSwap(t testkit.SeedT, deps module.Deps, cabal testkit.SeededCabal, e events.TradeSubmitted,
	at time.Time,
) {
	q := trading.New(deps.Pool)
	failSeed(t, q.InsertCreated(t.Context(), trading.InsertCreatedParams{
		ID: e.SwapID, SourceKind: e.Source.Kind, SourceID: e.Source.ID, CabalID: e.CabalID,
		TreasuryAddress: string(cabal.TreasuryAddress), Action: e.Action, Symbol: e.Symbol,
		InMint: string(e.InMint), OutMint: string(e.OutMint), OutDecimals: 8, InAmount: devProposalMicros,
		QuoteOutAmount: pgtype.Int8{Int64: devProposalQuote, Valid: true}, SlippageBps: 100, SourceBatchSize: 1,
		CreatedAt: at,
	}), "insert swap")
	_, err := q.MarkSubmitted(t.Context(), trading.MarkSubmittedParams{
		ID: e.SwapID, ExecuteRequestID: "dev-" + e.SwapID.String(), SignedTx: []byte{0},
		TxSignature: string(e.TxSignature), SubmittedAt: at,
	})
	failSeed(t, err, "submit swap")
	_, err = q.FinishFailed(t.Context(), trading.FinishFailedParams{
		ID: e.SwapID, FailureCode: "jupiter_failed", FailedAt: at,
	})
	failSeed(t, err, "fail swap")
}

const (
	devProposalMicros = 5_000_000
	devProposalQuote  = 21_000_000
)

const devProposalMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func newDevCabal(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) testkit.SeededCabal {
	return testkit.NewCabal(t, deps.Pool, testkit.WithCreator(actor("A")),
		testkit.WithJoiner(actor("B")), testkit.WithJoiner(actor("C")))
}

func seedUserWithBalance(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	raw := devScenarioJSONL(t, "user-with-balance", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a80b2": actor("A").String(),
	})
	out := map[string]string{}
	for _, s := range testkit.SeedJSONL(t, deps.Pool, "user-with-balance", raw, treasury.New(deps).Consumers()...) {
		if deposit, ok := s.Event.(events.DepositCredited); ok {
			out["deposit_id"], out["amount_micros"] = deposit.DepositID.String(), deposit.AmountMicros.String()
		}
	}
	return out
}

var (
	uuidPattern      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	signaturePattern = regexp.MustCompile(`"tx_signature":"([1-9A-HJ-NP-Za-km-z]+)"`)
)

func devScenarioJSONL(t testkit.SeedT, name string, keep map[string]string) []byte {
	t.Helper()
	raw, err := testkit.Scenario(name)
	failSeed(t, err, "scenario "+name)
	fresh := map[string]string{}
	for _, id := range uuidPattern.FindAll(raw, -1) {
		if _, ok := fresh[string(id)]; !ok {
			fresh[string(id)] = ids.Real{}.NewV7().String()
		}
	}
	for _, m := range signaturePattern.FindAllSubmatch(raw, -1) {
		sig := make([]byte, 64)
		_, _ = rand.Read(sig)
		fresh[string(m[1])] = string(chain.SignatureOf(sig))
	}
	for old, id := range keep {
		fresh[old] = id
	}
	for old, id := range fresh {
		raw = bytes.ReplaceAll(raw, []byte(old), []byte(id))
	}
	return raw
}

func failSeed(t testkit.SeedT, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}
