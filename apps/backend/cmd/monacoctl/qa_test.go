package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	qaUSDC   = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	qaMember = chain.SolanaAddress("Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")
)

type stubQARPC struct {
	lamports, micros uint64
	statuses         []solana.State
	failed           bool
	height           uint64
	sent             [][]byte
	decimals         uint8
	answer           chain.Signature
	fail             map[string]error
}

func (s *stubQARPC) SOLBalance(context.Context, chain.SolanaAddress) (money.BaseUnits, error) {
	return money.NewBaseUnits(s.lamports, 9), s.fail["SOLBalance"]
}

func (s *stubQARPC) TokenBalance(_ context.Context, _ chain.SolanaAddress, m chain.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(s.micros, m.Decimals), s.fail["TokenBalance"]
}

func (s *stubQARPC) MintConfig(_ context.Context, mint chain.SolanaAddress) (solana.MintConfig, error) {
	return solana.MintConfig{
		Mint: chain.Mint{Address: mint, Decimals: s.decimals}, TokenProgram: chain.SPLProgram,
	}, s.fail["MintConfig"]
}

func (s *stubQARPC) LatestBlockhash(context.Context) (solana.Blockhash, error) {
	return solana.Blockhash{Hash: [32]byte{9}, LastValidBlockHeight: 100}, s.fail["LatestBlockhash"]
}

func (s *stubQARPC) SendTransaction(_ context.Context, raw []byte) (chain.Signature, error) {
	if err := s.fail["SendTransaction"]; err != nil {
		return "", err
	}
	s.sent = append(s.sent, raw)
	tx, err := chain.DecodeTransaction(raw)
	if err != nil || s.answer != "" {
		return s.answer, err
	}
	return chain.SignatureOf(tx.Signatures[0]), nil
}

func (s *stubQARPC) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	if err := s.fail["SignatureStatuses"]; err != nil {
		return nil, err
	}
	state := s.statuses[0]
	if len(s.statuses) > 1 {
		s.statuses = s.statuses[1:]
	}
	return []solana.Status{{Signature: sigs[0], State: state, Failed: s.failed, BlockHeight: s.height}}, nil
}

type qaHarness struct {
	rpc     *stubQARPC
	pot     qaPot
	env     qaEnv
	lookups []uuid.UUID
}

func newQAHarness(t *testing.T) *qaHarness {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h := &qaHarness{
		rpc: &stubQARPC{
			lamports: 50_000_000, micros: 20_000_000, height: 50, decimals: 6,
			statuses: []solana.State{solana.StateProcessing},
		},
		pot: qaPot{key: key, address: chain.AddressOf(key.Public().(ed25519.PublicKey))},
	}
	h.env = qaEnv{
		environ: []string{
			"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco", "NATS_URL=nats://127.0.0.1:1",
			qaPotKey + "=" + chain.EncodeBase58(key),
		},
		root: t.TempDir(),
		rpc:  func(config.Config) qaRPC { return h.rpc },
		wallet: func(_ context.Context, _ config.Config, user uuid.UUID) (chain.SolanaAddress, error) {
			h.lookups = append(h.lookups, user)
			return qaMember, nil
		},
		setKey: func(context.Context, string, string) error {
			t.Fatal("setKey called")
			return nil
		},
		wait: time.Minute,
	}
	return h
}

func (h *qaHarness) run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := qaTool(h.env)(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

type qaInstruction struct {
	program  chain.SolanaAddress
	accounts []chain.SolanaAddress
	data     []byte
}

func decodeQAMessage(t *testing.T, msg []byte) ([3]byte, []chain.SolanaAddress, []qaInstruction) {
	t.Helper()
	next := func(n int) []byte {
		if len(msg) < n {
			t.Fatal("message cut short")
		}
		out := msg[:n]
		msg = msg[n:]
		return out
	}
	header := [3]byte(next(3))
	keys := make([]chain.SolanaAddress, next(1)[0])
	for i := range keys {
		keys[i] = chain.AddressOf(next(32))
	}
	next(32)
	ixs := make([]qaInstruction, next(1)[0])
	for i := range ixs {
		ixs[i].program = keys[next(1)[0]]
		for _, a := range next(int(next(1)[0])) {
			ixs[i].accounts = append(ixs[i].accounts, keys[a])
		}
		ixs[i].data = next(int(next(1)[0]))
	}
	if len(msg) != 0 {
		t.Fatalf("%d trailing bytes", len(msg))
	}
	return header, keys, ixs
}

func TestQAFund_sendsTheExactAmountFromThePotToTheUsersMemberWallet(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	user := uuid.New()
	h.rpc.statuses = []solana.State{solana.StateNotFound, solana.StateProcessing}

	code, stdout, stderr := h.run("fund", "--user", user.String(), "--usdc", "1.25")

	if code != 0 || len(h.rpc.sent) != 1 {
		t.Fatalf("fund = %d with %d sends, stderr %q; want 0 and one send", code, len(h.rpc.sent), stderr)
	}
	if len(h.lookups) != 1 || h.lookups[0] != user {
		t.Fatalf("looked up %v, want only %s", h.lookups, user)
	}
	sig := string(checkPotTransfer(t, h.rpc.sent[0], h.pot.address, 1_250_000))
	if !strings.Contains(stdout, sig) || !strings.Contains(stdout, string(qaMember)) {
		t.Fatalf("stdout %q, want the signature %s and the destination", stdout, sig)
	}
}

func checkPotTransfer(t *testing.T, raw []byte, pot chain.SolanaAddress, micros uint64) chain.Signature {
	t.Helper()
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Signatures) != 1 || tx.Signers[0] != pot || !tx.Signed(0) {
		t.Fatalf("signers %v, want the pot %s as the one signed signer", tx.Signers, pot)
	}
	header, keys, ixs := decodeQAMessage(t, tx.Message)
	if header != [3]byte{1, 0, 5} || keys[0] != pot {
		t.Fatalf("header %v fee payer %s, want [1 0 5] and the pot", header, keys[0])
	}
	source, _ := chain.AssociatedTokenAccount(pot, qaUSDC, chain.SPLProgram)
	dest, _ := chain.AssociatedTokenAccount(qaMember, qaUSDC, chain.SPLProgram)
	if len(ixs) != 2 {
		t.Fatalf("%d instructions, want createIdempotent then transferChecked", len(ixs))
	}
	create := qaInstruction{chain.ATAProgram, []chain.SolanaAddress{
		pot, dest, qaMember, qaUSDC, chain.SystemProgram, chain.SPLProgram,
	}, []byte{1}}
	transfer := qaInstruction{
		chain.SPLProgram,
		[]chain.SolanaAddress{source, qaUSDC, dest, pot},
		append(binary.LittleEndian.AppendUint64([]byte{12}, micros), 6),
	}
	for i, want := range []qaInstruction{create, transfer} {
		got := ixs[i]
		if got.program != want.program || !slices.Equal(got.accounts, want.accounts) ||
			!bytes.Equal(got.data, want.data) {
			t.Fatalf("instruction %d = %+v, want %+v", i, got, want)
		}
	}
	return chain.SignatureOf(tx.Signatures[0])
}

func TestQAFund_fiveUSDCIsTheLargestTransfer(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	if code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--usdc", "5"); code != 0 || len(h.rpc.sent) != 1 {
		t.Fatalf("fund 5 USDC = %d, %d sends, stderr %q; want one send", code, len(h.rpc.sent), stderr)
	}
	checkPotTransfer(t, h.rpc.sent[0], h.pot.address, 5_000_000)
}

func TestQAFund_refusesAndSendsNothing(t *testing.T) {
	t.Parallel()
	unknown := uuid.New()
	cases := map[string]struct {
		usdc  string
		setup func(*qaHarness)
		want  string
	}{
		"unknown user": {"1", func(h *qaHarness) {
			h.env.wallet = func(context.Context, config.Config, uuid.UUID) (chain.SolanaAddress, error) {
				return "", errNoMemberWallet
			}
		}, "no member wallet for user " + unknown.String()},
		"over 5 USDC":       {"5.000001", nil, "at most 5 USDC per transfer"},
		"6 USDC":            {"6", nil, "at most 5 USDC per transfer"},
		"zero":              {"0", nil, "must be more than 0 USDC"},
		"negative":          {"-1", nil, "must be more than 0 USDC"},
		"too many decimals": {"0.0000001", nil, "with at most 6 decimals"},
		"pot SOL low": {
			"1", func(h *qaHarness) { h.rpc.lamports = 9_999_999 },
			"holds 0.009999999 SOL and 20.000000 USDC",
		},
		"pot USDC low": {"2", func(h *qaHarness) { h.rpc.micros = 1_999_999 }, "The operator must top it up"},
		"production": {"1", func(h *qaHarness) {
			h.env.environ[0] = "MONACO_ENV=production"
		}, "refusing to run with MONACO_ENV=production"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newQAHarness(t)
			if c.setup != nil {
				c.setup(h)
			}
			code, _, stderr := h.run("fund", "--user", unknown.String(), "--usdc", c.usdc)
			if code == 0 || len(h.rpc.sent) != 0 || !strings.Contains(stderr, c.want) {
				t.Fatalf("fund = %d with %d sends, stderr %q; want non-zero, no send, %q",
					code, len(h.rpc.sent), stderr, c.want)
			}
		})
	}
}

func TestQAFund_potMessageNamesThePotAddress(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.rpc.micros = 0
	if _, _, stderr := h.run(
		"fund",
		"--user",
		uuid.NewString(),
		"--usdc",
		"1",
	); !strings.Contains(
		stderr,
		string(h.pot.address),
	) {
		t.Fatalf("stderr %q, want the pot address %s", stderr, h.pot.address)
	}
}

func TestQAFund_failsWhenTheTransferDoesNotLand(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		state  solana.State
		failed bool
		height uint64
		want   string
	}{
		"failed on chain": {solana.StateProcessing, true, 50, "failed on chain"},
		"expired":         {solana.StateNotFound, false, 101, "expired before it landed"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newQAHarness(t)
			h.rpc.statuses, h.rpc.failed, h.rpc.height = []solana.State{c.state}, c.failed, c.height
			if code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--usdc", "1"); code != 1 ||
				!strings.Contains(stderr, c.want) {
				t.Fatalf("fund = %d, stderr %q, want 1 and %q", code, stderr, c.want)
			}
		})
	}
}

func TestQAFund_needsAUserUUIDAndAnAmount(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"fund", "--usdc", "1"},
		{"fund", "--user", "not-a-uuid", "--usdc", "1"},
		{"fund", "--user", uuid.NewString()},
		{"fund", "--user", uuid.NewString(), "--usdc", "1", "--to", string(qaMember)},
	} {
		h := newQAHarness(t)
		if code, _, stderr := h.run(args...); code != 2 || !strings.Contains(stderr, qaUsage) || len(h.rpc.sent) != 0 {
			t.Fatalf("%v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestQAPot_printsTheAddressAndBothBalances(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	code, stdout, _ := h.run("pot")
	want := "address " + string(h.pot.address) + "\nsol 0.050000000\nusdc 20.000000\n"
	if code != 0 || stdout != want {
		t.Fatalf("pot = %d, %q; want %q", code, stdout, want)
	}
	code, stdout, _ = h.run("pot", "--address")
	if code != 0 || stdout != string(h.pot.address)+"\n" {
		t.Fatalf("pot --address = %d, %q; want only the address", code, stdout)
	}
}

func TestQAPot_withoutAKeySaysHowToGetOne(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.env.environ = h.env.environ[:3]
	if code, _, stderr := h.run("pot", "--address"); code != 1 || !strings.Contains(stderr, "qa pot new") {
		t.Fatalf("pot without a key = %d, stderr %q", code, stderr)
	}
}

func TestQAPotNew_writesAnEncryptedKeyAndPrintsOnlyTheAddress(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.env.environ = h.env.environ[:3]
	writeFile(t, h.env.root, ".env.local", "DOTENV_PUBLIC_KEY_LOCAL=\"pub\"\n")
	var written string
	h.env.setKey = func(_ context.Context, root, value string) error {
		written = value
		writeFile(t, root, ".env.local", "QA_POT_PRIVATE_KEY=\"encrypted:abc\"\n")
		return nil
	}

	code, stdout, stderr := h.run("pot", "new")

	pot, err := loadPot([]string{qaPotKey + "=" + written})
	if code != 0 || err != nil || stdout != string(pot.address)+"\n" {
		t.Fatalf("pot new = %d, stdout %q, stderr %q, load %v; want the new key's address", code, stdout, stderr, err)
	}
	if strings.Contains(stdout+stderr, written) {
		t.Fatal("pot new printed the private key")
	}
}

func TestQAPotNew_refusesToReplaceAKey(t *testing.T) {
	t.Parallel()
	inFile := newQAHarness(t)
	inFile.env.environ = inFile.env.environ[:3]
	writeFile(t, inFile.env.root, ".env.local", "A=1\nQA_POT_PRIVATE_KEY=\"encrypted:abc\"\n")
	inEnv := newQAHarness(t)
	for name, h := range map[string]*qaHarness{"in .env.local": inFile, "in the environment": inEnv} {
		if code, stdout, stderr := h.run("pot", "new"); code != 1 || stdout != "" ||
			!strings.Contains(stderr, "already set") {
			t.Fatalf("%s: pot new = %d, stdout %q, stderr %q; want a refusal", name, code, stdout, stderr)
		}
	}
}

func TestQAPotNew_flagsAPlaintextKey(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.env.environ = h.env.environ[:3]
	h.env.setKey = func(_ context.Context, root, value string) error {
		writeFile(t, root, ".env.local", qaPotKey+"="+value+"\n")
		return nil
	}
	if code, stdout, stderr := h.run(
		"pot",
		"new",
	); code != 1 || stdout != "" ||
		!strings.Contains(stderr, "not encrypted") {
		t.Fatalf("pot new with a plaintext write = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestMemberWallet_readsTheUsersWalletAndNamesAMissingOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := identity.New(pool)
	user := uuid.New()
	now := clock.Real{}.Now().UTC()
	if _, err := q.CreateUser(t.Context(), identity.CreateUserParams{
		ID: user, PrivyUserID: "did:privy:qa-" + user.String(), LoginProvider: "sms", Now: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AttachUserWallet(t.Context(), identity.AttachUserWalletParams{
		UserID: user, PrivyWalletID: "wallet-" + user.String(), Address: string(qaMember), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load([]string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := memberWallet(t.Context(), cfg, user); err != nil || got != qaMember {
		t.Fatalf("memberWallet = %s, %v; want %s", got, err, qaMember)
	}
	if _, err := memberWallet(t.Context(), cfg, uuid.New()); !errors.Is(err, errNoMemberWallet) {
		t.Fatalf("memberWallet for an unknown user = %v, want errNoMemberWallet", err)
	}
}

func TestQAFund_reportsEachRPCFailure(t *testing.T) {
	t.Parallel()
	for _, method := range []string{
		"MintConfig", "SOLBalance", "TokenBalance", "LatestBlockhash", "SendTransaction", "SignatureStatuses",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			h := newQAHarness(t)
			h.rpc.fail = map[string]error{method: errors.New("rpc down " + method)}
			if code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--usdc", "1"); code != 1 ||
				!strings.Contains(stderr, "rpc down "+method) {
				t.Fatalf("fund with %s failing = %d, stderr %q", method, code, stderr)
			}
		})
	}
}

func TestQAFund_refusesWhatItCannotTrust(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		setup func(*qaHarness)
		want  string
	}{
		"mint decimals":     {func(h *qaHarness) { h.rpc.decimals = 9 }, "has 9 decimals, want 6"},
		"another signature": {func(h *qaHarness) { h.rpc.answer = "other" }, "RPC answered signature other"},
		"wallet lookup fails": {func(h *qaHarness) {
			h.env.wallet = func(context.Context, config.Config, uuid.UUID) (chain.SolanaAddress, error) {
				return "", errors.New("db down")
			}
		}, "db down"},
		"no pot key": {func(h *qaHarness) { h.env.environ = h.env.environ[:3] }, "qa pot new"},
		"never confirms": {func(h *qaHarness) {
			h.rpc.statuses = []solana.State{solana.StateNotFound}
			h.env.poll, h.env.wait = time.Millisecond, 20*time.Millisecond
		}, "not confirmed in 20ms"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newQAHarness(t)
			c.setup(h)
			if code, _, stderr := h.run("fund", "--user", uuid.NewString(), "--usdc", "1"); code != 1 ||
				!strings.Contains(stderr, c.want) {
				t.Fatalf("fund = %d, stderr %q, want 1 and %q", code, stderr, c.want)
			}
		})
	}
}

func TestQAPot_reportsRPCFailuresAndBadFlags(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"SOLBalance", "TokenBalance"} {
		h := newQAHarness(t)
		h.rpc.fail = map[string]error{method: errors.New("rpc down")}
		if code, _, stderr := h.run("pot"); code != 1 || !strings.Contains(stderr, "rpc down") {
			t.Fatalf("pot with %s failing = %d, stderr %q", method, code, stderr)
		}
	}
	h := newQAHarness(t)
	if code, _, stderr := h.run("pot", "--bogus"); code != 2 || !strings.Contains(stderr, qaUsage) {
		t.Fatalf("pot --bogus = %d, stderr %q", code, stderr)
	}
}

func TestLoadPot_refusesAKeyWhosePublicHalfIsWrong(t *testing.T) {
	t.Parallel()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bad := slices.Clone(key)
	bad[63] ^= 1
	if _, err := loadPot([]string{qaPotKey + "=" + chain.EncodeBase58(bad)}); !errors.Is(err, errQAPotKey) {
		t.Fatalf("loadPot = %v, want errQAPotKey", err)
	}
}

func TestQAPotNew_reportsAFailedWrite(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	h.env.environ = h.env.environ[:3]
	h.env.setKey = func(context.Context, string, string) error { return errors.New("dotenvx missing") }
	if code, stdout, stderr := h.run("pot", "new"); code != 1 || stdout != "" ||
		!strings.Contains(stderr, "dotenvx missing") {
		t.Fatalf("pot new = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestDotenvxSet_runsSetInTheRootAndHidesTheKeyWhenItFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := dotenvxSet(qaTestdata(t, "qa-dotenvx-ok.sh"))(t.Context(), root, "secret"); err != nil {
		t.Fatal(err)
	}
	if !envLocalHas(root, "set QA_POT_PRIVATE_KEY secret -f .env.local") {
		t.Fatal("dotenvx did not get set QA_POT_PRIVATE_KEY <key> -f .env.local in the root")
	}
	err := dotenvxSet(qaTestdata(t, "qa-dotenvx-fail.sh"))(t.Context(), root, "secret")
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "<key>") {
		t.Fatalf("failed set = %v, want an error with the key hidden", err)
	}
}

func qaTestdata(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestToolQa_findsTheRepoRootAndUsesTheRealDependencies(t *testing.T) {
	t.Parallel()
	h := newQAHarness(t)
	var stdout, stderr bytes.Buffer
	if code := toolQa(toolEnv{environ: h.env.environ, wd: t.TempDir()})(
		[]string{"pot", "--address"}, &stdout, &stderr,
	); code != 0 || stdout.String() != string(h.pot.address)+"\n" {
		t.Fatalf("qa pot --address = %d, %q, %q", code, stdout.String(), stderr.String())
	}
	environ := append(slices.Clone(h.env.environ), "SOLANA_RPC_URL=http://127.0.0.1:1")
	if code := toolQa(toolEnv{environ: environ, wd: t.TempDir()})([]string{"pot"}, &stdout, &stderr); code != 1 {
		t.Fatalf("qa pot against a closed RPC port = %d, want 1", code)
	}
	wd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if root := repoRoot(wd); root != filepath.Join(wd, "..", "..", "..", "..") {
		t.Fatalf("repoRoot(%s) = %s, want the checkout root", wd, root)
	}
	if outside := t.TempDir(); repoRoot(outside) != outside {
		t.Fatalf("repoRoot outside a checkout = %s, want %s", repoRoot(outside), outside)
	}
}

func TestMemberWallet_reportsAnUnreachableDatabaseAndAFailedQuery(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load([]string{
		"MONACO_ENV=test",
		"DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1",
		"NATS_URL=nats://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memberWallet(t.Context(), cfg, uuid.New()); err == nil {
		t.Fatal("memberWallet with no database succeeded")
	}
	if _, err := destination("", errors.New("query failed")); err == nil || errors.Is(err, errNoMemberWallet) {
		t.Fatalf("destination with a query error = %v, want that error", err)
	}
}
