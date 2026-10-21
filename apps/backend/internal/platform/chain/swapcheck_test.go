package chain_test

import (
	"encoding/binary"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/swaptx"
)

func payer() chain.SolanaAddress    { return swaptx.Address("payer") }
func wallet() chain.SolanaAddress   { return swaptx.Address("wallet") }
func attacker() chain.SolanaAddress { return swaptx.Address("attacker") }

func swapUSDC() chain.Mint {
	return chain.Mint{Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6}
}

func swapStock() chain.Mint {
	return chain.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8}
}

func spec() swaptx.Spec {
	return swaptx.Spec{
		Payer: payer(), Wallet: wallet(), In: swapUSDC(), Out: swapStock(),
		InAmount: 25_000_000, QuotedOut: 11_000_000, SlippageBps: 50,
	}
}

func intent() chain.SwapIntent {
	return chain.SwapIntent{
		Payer: payer(), Wallet: wallet(), In: swapUSDC(), Out: swapStock(), MaxIn: 25_000_000, MinOut: 10_900_000,
		MaxFeeLamports: chain.MaxSwapFeeLamports,
	}
}

func verify(tx swaptx.Tx, in chain.SwapIntent) error {
	msg, ixs, err := chain.ResolveInstructions(tx.Bytes(), func([]chain.SolanaAddress) (chain.LookupTables, error) {
		return tx.Tables(), nil
	})
	if err != nil {
		return err
	}
	return chain.CheckSwap(msg, ixs, in)
}

func mustPass(t *testing.T, tx swaptx.Tx) {
	t.Helper()
	if err := verify(tx, intent()); err != nil {
		t.Fatalf("honest transaction refused: %v %v", err, errs.Detail(err))
	}
}

func mustRefuse(t *testing.T, tx swaptx.Tx, in chain.SwapIntent) {
	t.Helper()
	err := verify(tx, in)
	if err == nil {
		t.Fatal("malicious transaction was accepted")
	}
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("refusal code = %s (%v), want invalid_input", errs.CodeOf(err), err)
	}
}

func sharedAt(tx swaptx.Tx, i int, with swaptx.Ix) swaptx.Tx {
	tx.Ixs = append([]swaptx.Ix(nil), tx.Ixs...)
	tx.Ixs[i] = with
	return tx
}

func routeIndex(tx swaptx.Tx) int {
	for i, ix := range tx.Ixs {
		if ix.Program == chain.JupiterV6Program {
			return i
		}
	}
	return -1
}

func tabled(tx swaptx.Tx) swaptx.Tx {
	seen := map[chain.SolanaAddress]bool{tx.Payer: true, tx.Wallet: true}
	for _, ix := range tx.Ixs {
		seen[ix.Program] = true
	}
	tx.Table = swaptx.Address("lookup-table")
	tx.Tabled = []chain.SolanaAddress{swaptx.Address("padding")}
	for _, ix := range tx.Ixs {
		for _, a := range ix.Accounts {
			if !seen[a] {
				seen[a] = true
				tx.Tabled = append(tx.Tabled, a)
			}
		}
	}
	return tx
}

func systemTransfer(from, to chain.SolanaAddress, lamports uint64) swaptx.Ix {
	data := binary.LittleEndian.AppendUint32(nil, 2)
	return swaptx.Ix{
		Program: chain.SystemProgram, Accounts: []chain.SolanaAddress{from, to},
		Data: binary.LittleEndian.AppendUint64(data, lamports),
	}
}

func tokenIx(tag byte, accounts ...chain.SolanaAddress) swaptx.Ix {
	return swaptx.Ix{Program: chain.SPLProgram, Accounts: accounts, Data: []byte{tag, 0, 0, 0, 0, 0, 0, 0, 0}}
}

func TestCheckSwap_honestTransactionsPass(t *testing.T) {
	t.Parallel()
	mustPass(t, swaptx.HonestTx(spec()))
	mustPass(t, tabled(swaptx.HonestTx(spec())))
	route := swaptx.HonestTx(spec())
	mustPass(t, sharedAt(route, routeIndex(route), swaptx.Route(spec())))
	mustPass(t, tabled(sharedAt(route, routeIndex(route), swaptx.Route(spec()))))
}

func TestCheckSwap_refusesMaliciousBytesWithValidLookingMetadata(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	at := routeIndex(honest)
	wrongOut, wrongIn := spec(), spec()
	wrongOut.Out = chain.Mint{Address: swaptx.Address("other-mint"), Decimals: 8}
	wrongIn.In = chain.Mint{Address: swaptx.Address("other-input"), Decimals: 6}
	stolen := swaptx.SharedRoute(spec())
	stolen.Accounts = append([]chain.SolanaAddress(nil), stolen.Accounts...)
	stolen.Accounts[6] = swaptx.ATA(attacker(), swapStock().Address)
	drained := swaptx.SharedRoute(spec())
	drained.Accounts = append([]chain.SolanaAddress(nil), drained.Accounts...)
	drained.Accounts[3] = swaptx.ATA(attacker(), swapUSDC().Address)
	otherAuthority := swaptx.SharedRoute(spec())
	otherAuthority.Accounts = append([]chain.SolanaAddress(nil), otherAuthority.Accounts...)
	otherAuthority.Accounts[2] = attacker()
	over := spec()
	over.InAmount = 25_000_001
	thin := spec()
	thin.SlippageBps = 1_000
	fees := spec()
	feeIx := swaptx.SharedRoute(fees)
	feeIx.Data = append([]byte(nil), feeIx.Data...)
	feeIx.Data[len(feeIx.Data)-1] = chain.MaxPlatformFeeBps + 1
	wrongHop := swaptx.Route(spec())
	wrongHop.Accounts = append([]chain.SolanaAddress(nil), wrongHop.Accounts...)
	wrongHop.Accounts[4] = swaptx.ATA(attacker(), swapStock().Address)
	cases := map[string]swaptx.Tx{
		"wrong output mint":          sharedAt(honest, at, swaptx.SharedRoute(wrongOut)),
		"wrong input mint":           sharedAt(honest, at, swaptx.SharedRoute(wrongIn)),
		"wrong recipient":            sharedAt(honest, at, stolen),
		"wrong recipient in a table": tabled(sharedAt(honest, at, stolen)),
		"drains another account":     sharedAt(honest, at, drained),
		"spends as another signer":   sharedAt(honest, at, otherAuthority),
		"amount above the ceiling":   sharedAt(honest, at, swaptx.SharedRoute(over)),
		"minimum below the floor":    sharedAt(honest, at, swaptx.SharedRoute(thin)),
		"platform fee above ceiling": sharedAt(honest, at, feeIx),
		"second destination account": sharedAt(honest, at, wrongHop),
		"no route":                   {Payer: payer(), Wallet: wallet(), Ixs: honest.Ixs[:at]},
	}
	extra := func(ix swaptx.Ix) swaptx.Tx {
		tx := honest
		tx.Ixs = append(append([]swaptx.Ix(nil), honest.Ixs[:at]...), ix, honest.Ixs[at])
		return tx
	}
	cases["extra system transfer from the treasury"] = extra(systemTransfer(wallet(), attacker(), 1))
	cases["extra system transfer above the ceiling"] = extra(
		systemTransfer(payer(), attacker(), chain.MaxSystemLamports+1),
	)
	cases["extra token transfer"] = extra(
		tokenIx(3, swaptx.ATA(wallet(), swapUSDC().Address), swaptx.ATA(attacker(), swapUSDC().Address), wallet()),
	)
	cases["extra token transfer checked"] = extra(
		tokenIx(
			12,
			swaptx.ATA(wallet(), swapUSDC().Address),
			swapUSDC().Address,
			swaptx.ATA(attacker(), swapUSDC().Address),
			wallet(),
		),
	)
	cases["authority change"] = extra(tokenIx(6, swaptx.ATA(wallet(), swapUSDC().Address), wallet()))
	cases["close account"] = extra(tokenIx(9, swaptx.ATA(wallet(), swapUSDC().Address), attacker(), wallet()))
	cases["delegate approval"] = extra(tokenIx(4, swaptx.ATA(wallet(), swapUSDC().Address), attacker(), wallet()))
	cases["token account for a third party"] = extra(
		tokenIx(18, swaptx.ATA(attacker(), swapUSDC().Address), swapUSDC().Address),
	)
	cases["unknown program"] = extra(
		swaptx.Ix{Program: swaptx.Address("drainer"), Accounts: []chain.SolanaAddress{wallet()}, Data: []byte{1}},
	)
	cases["second route"] = extra(swaptx.SharedRoute(spec()))
	for name, tx := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mustRefuse(t, tx, intent())
		})
	}
}

func TestCheckSwap_refusesOtherAssociatedAccountsAndSystemFunding(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	ata := func(funder, owner, mint chain.SolanaAddress, data []byte) swaptx.Ix {
		return swaptx.Ix{
			Program: chain.ATAProgram, Data: data,
			Accounts: []chain.SolanaAddress{
				funder, swaptx.ATA(owner, mint), owner, mint, chain.SystemProgram, chain.SPLProgram,
			},
		}
	}
	put := func(ix swaptx.Ix) swaptx.Tx {
		tx := honest
		tx.Ixs = append(append([]swaptx.Ix(nil), honest.Ixs[:2]...), ix, honest.Ixs[3])
		return tx
	}
	forged := ata(payer(), wallet(), swapStock().Address, []byte{1})
	forged.Accounts = append([]chain.SolanaAddress(nil), forged.Accounts...)
	forged.Accounts[1] = swaptx.Address("not-derived")
	otherProgram := ata(payer(), wallet(), swapStock().Address, []byte{1})
	otherProgram.Accounts = append([]chain.SolanaAddress(nil), otherProgram.Accounts...)
	otherProgram.Accounts[5] = swaptx.Address("fake-token-program")
	for name, tx := range map[string]swaptx.Tx{
		"create for an attacker":      put(ata(payer(), attacker(), swapStock().Address, []byte{1})),
		"treasury funds the account":  put(ata(wallet(), wallet(), swapStock().Address, []byte{1})),
		"mint outside the trade":      put(ata(payer(), wallet(), swaptx.Address("junk-mint"), []byte{1})),
		"not the derived address":     put(forged),
		"not a token program":         put(otherProgram),
		"recover nested instruction":  put(ata(payer(), wallet(), swapStock().Address, []byte{2})),
		"trailing data":               put(ata(payer(), wallet(), swapStock().Address, []byte{1, 1})),
		"short account list":          put(swaptx.Ix{Program: chain.ATAProgram, Data: []byte{1}, Accounts: []chain.SolanaAddress{payer()}}),
		"system create from treasury": put(systemCreate(wallet(), attacker())),
		"system assign":               put(swaptx.Ix{Program: chain.SystemProgram, Accounts: []chain.SolanaAddress{payer(), attacker()}, Data: append(binary.LittleEndian.AppendUint32(nil, 1), make([]byte, 8)...)}),
		"system too short":            put(swaptx.Ix{Program: chain.SystemProgram, Accounts: []chain.SolanaAddress{payer()}, Data: []byte{2}}),
		"token instruction empty":     put(swaptx.Ix{Program: chain.SPLProgram}),
		"initialize owned by thief":   put(tokenIx(1, swaptx.ATA(wallet(), swapUSDC().Address), swapUSDC().Address, attacker())),
		"initialize too short":        put(tokenIx(1, swaptx.ATA(wallet(), swapUSDC().Address))),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mustRefuse(t, tx, intent())
		})
	}
	mustPass(t, put(ata(payer(), wallet(), swapStock().Address, nil)))
	mustPass(t, put(ata(payer(), wallet(), swapUSDC().Address, []byte{0})))
	mustPass(t, put(systemCreate(payer(), swaptx.Address("new-account"))))
	mustPass(t, put(tokenIx(17, swaptx.ATA(wallet(), swapUSDC().Address))))
	mustPass(t, put(tokenIx(1, swaptx.ATA(wallet(), swapUSDC().Address), swapUSDC().Address, wallet())))
}

func systemCreate(from, to chain.SolanaAddress) swaptx.Ix {
	data := binary.LittleEndian.AppendUint32(nil, 0)
	data = binary.LittleEndian.AppendUint64(data, 2_039_280)
	return swaptx.Ix{Program: chain.SystemProgram, Accounts: []chain.SolanaAddress{from, to}, Data: data}
}

func TestCheckSwap_refusesTheWrongSigners(t *testing.T) {
	t.Parallel()
	tx := swaptx.HonestTx(spec())
	msg, ixs, err := chain.ResolveInstructions(tx.Bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(chain.SwapIntent) chain.SwapIntent{
		"fee payer is not the relayer": func(i chain.SwapIntent) chain.SwapIntent { i.Payer = attacker(); return i },
		"wallet is not the treasury":   func(i chain.SwapIntent) chain.SwapIntent { i.Wallet = attacker(); return i },
		"payer is the treasury":        func(i chain.SwapIntent) chain.SwapIntent { i.Payer = i.Wallet; return i },
	} {
		if err := chain.CheckSwap(msg, ixs, mutate(intent())); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
	msg.Keys = append(msg.Keys, attacker())
	msg.Signers = 3
	if err := chain.CheckSwap(msg, ixs, intent()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("a third signer: err = %v, want invalid_input", err)
	}
}

func TestCheckSwap_boundsThePriorityFee(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	price := func(micro uint64) swaptx.Tx {
		tx := honest
		tx.Ixs = append([]swaptx.Ix(nil), honest.Ixs...)
		tx.Ixs[1] = swaptx.Budget(3, binary.LittleEndian.AppendUint64(nil, micro))
		return tx
	}
	mustPass(t, price(25_000_000))
	mustRefuse(t, price(25_000_001), intent())
	withLimit := func(units uint32) swaptx.Tx {
		tx := price(25_000_000)
		tx.Ixs[0] = swaptx.Budget(2, binary.LittleEndian.AppendUint32(nil, units))
		return tx
	}
	mustPass(t, withLimit(400_000))
	mustRefuse(t, withLimit(400_001), intent())
	unlimited := price(25_000_000)
	unlimited.Ixs = append([]swaptx.Ix(nil), unlimited.Ixs[1:]...)
	mustPass(t, unlimited)
	unlimited = price(25_000_001)
	unlimited.Ixs = append([]swaptx.Ix(nil), unlimited.Ixs[1:]...)
	mustRefuse(t, unlimited, intent())
	for name, ix := range map[string]swaptx.Ix{
		"deprecated request":    swaptx.Budget(0, make([]byte, 8)),
		"unknown tag":           swaptx.Budget(9, nil),
		"short limit":           swaptx.Budget(2, []byte{1}),
		"short price":           swaptx.Budget(3, []byte{1}),
		"empty instruction":     {Program: chain.ComputeBudgetProgram},
		"second unit limit set": swaptx.Budget(2, binary.LittleEndian.AppendUint32(nil, 1)),
	} {
		tx := honest
		tx.Ixs = append([]swaptx.Ix{ix}, honest.Ixs...)
		if err := verify(tx, intent()); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
	for _, tag := range []byte{1, 4} {
		tx := honest
		tx.Ixs = append([]swaptx.Ix{swaptx.Budget(tag, binary.LittleEndian.AppendUint32(nil, 1))}, honest.Ixs...)
		mustPass(t, tx)
	}
}

func TestCheckSwap_theInputCeilingIsExactAndTheMinimumIsDerivedFromTheBytes(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ceiling := rapid.Uint64Range(1, 1<<50).Draw(t, "ceiling")
		in := rapid.Uint64Range(1, 1<<51).Draw(t, "in")
		s := spec()
		s.InAmount, s.QuotedOut, s.SlippageBps = in, 1<<40, 100
		allowed := intent()
		allowed.MaxIn, allowed.MinOut = ceiling, 0
		err := verify(swaptx.HonestTx(s), allowed)
		if (in <= ceiling) != (err == nil) {
			t.Fatalf("input %d against ceiling %d: err = %v", in, ceiling, err)
		}
		floor := rapid.Uint64Range(0, 1<<41).Draw(t, "floor")
		allowed.MaxIn, allowed.MinOut = in, floor
		err = verify(swaptx.HonestTx(s), allowed)
		minOut := (uint64(1<<40) * 9_900) / 10_000
		if (minOut >= floor) != (err == nil) {
			t.Fatalf("derived minimum %d against floor %d: err = %v", minOut, floor, err)
		}
	})
}

func TestScreenPrograms_refusesTokenInstructionsOutsideTheSetup(t *testing.T) {
	t.Parallel()
	for name, ix := range map[string]chain.Instruction{
		"set authority": {Program: chain.SPLProgram, Data: []byte{6}},
		"close account": {Program: chain.SPL2022Program, Data: []byte{9}},
		"empty":         {Program: chain.SPLProgram},
		"unknown":       {Program: swaptx.Address("drainer"), Data: []byte{1}},
	} {
		if err := chain.ScreenPrograms([]chain.Instruction{ix}); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
	if err := chain.ScreenPrograms([]chain.Instruction{
		{Program: chain.SPL2022Program, Data: []byte{17}}, {Program: chain.JupiterV6Program},
	}); err != nil {
		t.Fatalf("setup instructions refused: %v", err)
	}
}

func TestJupiterRoute_readsTheAmountsFromTheTail(t *testing.T) {
	t.Parallel()
	for _, ix := range []swaptx.Ix{swaptx.SharedRoute(spec()), swaptx.Route(spec())} {
		route, ok := chain.JupiterRoute(chain.Instruction{Program: chain.JupiterV6Program, Data: ix.Data})
		if !ok || route.In != 25_000_000 || route.QuotedOut != 11_000_000 || route.SlippageBps != 50 ||
			route.FeeBps != 0 {
			t.Fatalf("route = %+v, %v", route, ok)
		}
		if got := route.MinOut(); got != 10_945_000 {
			t.Fatalf("MinOut = %d, want 10945000", got)
		}
	}
	for name, ix := range map[string]chain.Instruction{
		"other program": {Program: chain.SystemProgram, Data: swaptx.SharedRoute(spec()).Data},
		"short":         {Program: chain.JupiterV6Program, Data: []byte{1, 2, 3}},
		"other method":  {Program: chain.JupiterV6Program, Data: make([]byte, 40)},
	} {
		if _, ok := chain.JupiterRoute(ix); ok {
			t.Fatalf("%s read as a route", name)
		}
	}
	fee := chain.SwapRoute{QuotedOut: 1_000_000, SlippageBps: 100, FeeBps: 100}
	if got := fee.MinOut(); got != 980_100 {
		t.Fatalf("MinOut with a fee = %d, want 980100", got)
	}
}

func withAccount(ix swaptx.Ix, at int, to chain.SolanaAddress) swaptx.Ix {
	ix.Accounts = append([]chain.SolanaAddress(nil), ix.Accounts...)
	ix.Accounts[at] = to
	return ix
}

func TestCheckSwap_refusesRoutesWhoseOwnAccountsDisagreeWithTheTrade(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	at := routeIndex(honest)
	shared, plain := swaptx.SharedRoute(spec()), swaptx.Route(spec())
	other := swaptx.Address("other-mint")
	for name, ix := range map[string]swaptx.Ix{
		"shared output mint field": withAccount(shared, 8, other),
		"shared input mint field":  withAccount(shared, 7, other),
		"plain output mint field":  withAccount(plain, 5, other),
		"plain authority":          withAccount(plain, 1, other),
		"plain source account":     withAccount(plain, 2, swaptx.ATA(attacker(), swapUSDC().Address)),
		"plain destination":        withAccount(plain, 3, swaptx.ATA(attacker(), swapStock().Address)),
		"unknown route method":     {Program: chain.JupiterV6Program, Accounts: shared.Accounts, Data: make([]byte, 40)},
		"too few accounts":         {Program: chain.JupiterV6Program, Accounts: shared.Accounts[:5], Data: shared.Data},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mustRefuse(t, sharedAt(honest, at, ix), intent())
		})
	}
}

func TestCheckSwap_judgesWhoOwnsATokenAccountItInitializes(t *testing.T) {
	t.Parallel()
	honest := swaptx.HonestTx(spec())
	at := routeIndex(honest)
	initialize := func(tag byte, owner chain.SolanaAddress) swaptx.Tx {
		tx := honest
		raw, _ := owner.Bytes()
		ix := swaptx.Ix{
			Program: chain.SPLProgram, Accounts: []chain.SolanaAddress{swaptx.ATA(wallet(), swapUSDC().Address)},
			Data: append([]byte{tag}, raw...),
		}
		tx.Ixs = append(append([]swaptx.Ix(nil), honest.Ixs[:at]...), ix, honest.Ixs[at])
		return tx
	}
	for _, tag := range []byte{16, 18} {
		mustPass(t, initialize(tag, wallet()))
		mustPass(t, initialize(tag, payer()))
		mustRefuse(t, initialize(tag, attacker()), intent())
	}
}
