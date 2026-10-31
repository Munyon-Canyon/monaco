package chain

import (
	"encoding/binary"
	"log/slog"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	ComputeBudgetProgram SolanaAddress = "ComputeBudget111111111111111111111111111111"
	JupiterV6Program     SolanaAddress = "JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4"

	MaxSwapFeeLamports   = 10_000_000
	MaxSystemLamports    = 10_000_000
	MaxPlatformFeeBps    = 100
	defaultUnitsPerIx    = 200_000
	maxComputeUnits      = 1_400_000
	microLamportsPerUnit = 1_000_000
	bpsDenominator       = 10_000
	routeTail            = 8 + 8 + 2 + 1
	discriminatorSize    = 8
	lamportsOffset       = 4
	pubkeyLen            = 32
)

const (
	systemCreateAccount = 0
	systemTransfer      = 2

	tokenInitializeAccount  = 1
	tokenInitializeAccount2 = 16
	tokenSyncNative         = 17
	tokenInitializeAccount3 = 18

	ataCreate           = 0
	ataCreateIdempotent = 1

	budgetHeapFrame     = 1
	budgetUnitLimit     = 2
	budgetUnitPrice     = 3
	budgetAccountsLimit = 4
)

const (
	routeDiscriminator  = "\xe5\x17\xcb\x97\x7a\xe3\xad\x2a"
	sharedDiscriminator = "\xc1\x20\x9b\x33\x41\xd6\x9c\x81"
)

type SwapIntent struct {
	Payer          SolanaAddress
	Wallet         SolanaAddress
	In             Mint
	Out            Mint
	MaxIn          uint64
	MinOut         uint64
	MaxFeeLamports uint64
}

type SwapRoute struct {
	In          uint64
	QuotedOut   uint64
	SlippageBps uint16
	FeeBps      uint8
}

func (r SwapRoute) MinOut() uint64 {
	kept := new(big.Int).Mul(new(big.Int).SetUint64(r.QuotedOut), big.NewInt(bpsDenominator-int64(r.SlippageBps)))
	kept.Quo(kept, big.NewInt(bpsDenominator))
	kept.Mul(kept, big.NewInt(bpsDenominator-int64(r.FeeBps)))
	kept.Quo(kept, big.NewInt(bpsDenominator))
	return kept.Uint64()
}

func swapPrograms() []SolanaAddress {
	return []SolanaAddress{
		SystemProgram,
		SPLProgram,
		SPL2022Program,
		ATAProgram,
		JupiterV6Program,
		ComputeBudgetProgram,
	}
}

func tokenPrograms() []SolanaAddress { return []SolanaAddress{SPLProgram, SPL2022Program} }

func ScreenPrograms(ixs []Instruction) error {
	const op = "chain.ScreenPrograms"
	for _, ix := range ixs {
		if !slices.Contains(swapPrograms(), ix.Program) {
			return refusal(op, "program outside the swap allowlist", slog.String("program", string(ix.Program)))
		}
		if ix.Program == SPLProgram || ix.Program == SPL2022Program {
			if len(ix.Data) == 0 || !slices.Contains(allowedTokenTags(), ix.Data[0]) {
				return refusal(
					op,
					"token instruction outside the swap allowlist",
					slog.String("program", string(ix.Program)),
				)
			}
		}
	}
	return nil
}

func allowedTokenTags() []byte {
	return []byte{tokenInitializeAccount, tokenInitializeAccount2, tokenSyncNative, tokenInitializeAccount3}
}

func ResolveInstructions(
	raw []byte,
	tables func([]SolanaAddress) (LookupTables, error),
) (Message, []Instruction, error) {
	tx, err := DecodeTransaction(raw)
	if err != nil {
		return Message{}, nil, err
	}
	msg, err := ParseMessage(tx.Message)
	if err != nil {
		return Message{}, nil, err
	}
	var loaded LookupTables
	if len(msg.Lookups) > 0 {
		if loaded, err = tables(msg.Tables()); err != nil {
			return Message{}, nil, err
		}
	}
	ixs, err := msg.Resolve(loaded)
	return msg, ixs, err
}

func CheckSwap(msg Message, ixs []Instruction, intent SwapIntent) error {
	if err := ScreenPrograms(ixs); err != nil {
		return err
	}
	signing := msg.Signing()
	if len(signing) != 2 || signing[0] != intent.Payer || signing[1] != intent.Wallet || intent.Payer == intent.Wallet {
		return refusal("chain.CheckSwap", "signers are not the fee payer and the treasury wallet")
	}
	route, err := checkInstructions(ixs, intent)
	if err != nil {
		return err
	}
	if err := checkRouteAmounts(route, intent); err != nil {
		return err
	}
	return checkFee(ixs, intent)
}

func checkInstructions(ixs []Instruction, intent SwapIntent) (SwapRoute, error) {
	var routes []SwapRoute
	for _, ix := range ixs {
		var err error
		switch ix.Program {
		case SystemProgram:
			err = checkSystem(ix, intent)
		case SPLProgram, SPL2022Program:
			err = checkToken(ix, intent)
		case ATAProgram:
			err = checkATA(ix, intent)
		case JupiterV6Program:
			var route SwapRoute
			route, err = checkRoute(ix, intent)
			routes = append(routes, route)
		case ComputeBudgetProgram:
		}
		if err != nil {
			return SwapRoute{}, err
		}
	}
	if len(routes) != 1 {
		return SwapRoute{}, refusal("chain.CheckSwap", "swap needs exactly one Jupiter route instruction",
			slog.Int("routes", len(routes)))
	}
	return routes[0], nil
}

func checkRouteAmounts(route SwapRoute, intent SwapIntent) error {
	const op = "chain.CheckSwap"
	switch {
	case route.In == 0 || route.In > intent.MaxIn:
		return refusal(op, "input amount above the ceiling",
			slog.Uint64("in", route.In), slog.Uint64("ceiling", intent.MaxIn))
	case route.FeeBps > MaxPlatformFeeBps:
		return refusal(op, "platform fee above the ceiling", slog.Int("fee_bps", int(route.FeeBps)))
	case route.MinOut() < intent.MinOut:
		return refusal(op, "minimum output below the approved floor",
			slog.Uint64("min_out", route.MinOut()), slog.Uint64("floor", intent.MinOut))
	}
	return nil
}

func checkSystem(ix Instruction, intent SwapIntent) error {
	const op = "chain.CheckSwap.system"
	if len(ix.Data) < lamportsOffset+8 || len(ix.Accounts) < 2 {
		return refusal(op, "system instruction shape")
	}
	tag := binary.LittleEndian.Uint32(ix.Data)
	lamports := binary.LittleEndian.Uint64(ix.Data[lamportsOffset:])
	switch {
	case tag != systemCreateAccount && tag != systemTransfer:
		return refusal(op, "system instruction outside the swap allowlist", slog.Int("tag", int(tag)))
	case ix.Accounts[0] == intent.Wallet:
		return refusal(op, "treasury wallet funds a system instruction")
	case lamports > MaxSystemLamports:
		return refusal(op, "system lamports above the ceiling", slog.Uint64("lamports", lamports))
	}
	return nil
}

func checkToken(ix Instruction, intent SwapIntent) error {
	const op = "chain.CheckSwap.token"
	var owner SolanaAddress
	switch ix.Data[0] {
	case tokenInitializeAccount:
		if len(ix.Accounts) < 3 {
			return refusal(op, "initialize account shape")
		}
		owner = ix.Accounts[2]
	case tokenInitializeAccount2, tokenInitializeAccount3:
		if len(ix.Data) < 1+pubkeyLen {
			return refusal(op, "initialize account shape")
		}
		owner = AddressOf(ix.Data[1 : 1+pubkeyLen])
	default:
		return nil
	}
	if owner != intent.Wallet && owner != intent.Payer {
		return refusal(op, "token account owned by a third party", slog.String("owner", string(owner)))
	}
	return nil
}

func checkATA(ix Instruction, intent SwapIntent) error {
	const op = "chain.CheckSwap.ata"
	create := len(ix.Data) == 0 || len(ix.Data) == 1 && (ix.Data[0] == ataCreate || ix.Data[0] == ataCreateIdempotent)
	if !create || len(ix.Accounts) < 6 {
		return refusal(op, "associated token instruction outside the swap allowlist")
	}
	funder, account, owner, mint, tokenProgram := ix.Accounts[0], ix.Accounts[1], ix.Accounts[2], ix.Accounts[3], ix.Accounts[5]
	if !derivesFrom(account, owner, mint, tokenProgram) {
		return refusal(op, "associated token account does not derive from its owner and mint")
	}
	if owner != intent.Wallet || funder == intent.Wallet {
		return refusal(op, "associated token account is for another owner or funded by the treasury wallet")
	}
	if mint != intent.In.Address && mint != intent.Out.Address {
		return refusal(op, "associated token account for a mint outside the trade", slog.String("mint", string(mint)))
	}
	return nil
}

func derivesFrom(account, owner, mint, tokenProgram SolanaAddress) bool {
	want, err := AssociatedTokenAccount(owner, mint, tokenProgram)
	return err == nil && want == account && slices.Contains(tokenPrograms(), tokenProgram)
}

func JupiterRoute(ix Instruction) (SwapRoute, bool) {
	if ix.Program != JupiterV6Program || len(ix.Data) < discriminatorSize+routeTail {
		return SwapRoute{}, false
	}
	if disc := string(ix.Data[:discriminatorSize]); disc != routeDiscriminator && disc != sharedDiscriminator {
		return SwapRoute{}, false
	}
	tail := ix.Data[len(ix.Data)-routeTail:]
	return SwapRoute{
		In: binary.LittleEndian.Uint64(tail), QuotedOut: binary.LittleEndian.Uint64(tail[8:]),
		SlippageBps: binary.LittleEndian.Uint16(tail[16:]), FeeBps: tail[18],
	}, true
}

type routeAccounts struct {
	authority, source, destination, optionalDestination, destinationMint, sourceMint, count int
}

func layoutOf(ix Instruction) routeAccounts {
	if string(ix.Data[:discriminatorSize]) == sharedDiscriminator {
		return routeAccounts{
			authority:           2,
			source:              3,
			destination:         6,
			optionalDestination: -1,
			destinationMint:     8,
			sourceMint:          7,
			count:               13,
		}
	}
	return routeAccounts{
		authority: 1, source: 2, destination: 3, optionalDestination: 4, destinationMint: 5, sourceMint: -1, count: 9,
	}
}

func checkRoute(ix Instruction, intent SwapIntent) (SwapRoute, error) {
	const op = "chain.CheckSwap.route"
	route, ok := JupiterRoute(ix)
	if !ok {
		return SwapRoute{}, refusal(op, "Jupiter instruction is not a route")
	}
	layout := layoutOf(ix)
	if len(ix.Accounts) < layout.count {
		return SwapRoute{}, refusal(op, "route accounts")
	}
	if err := layout.check(ix.Accounts, intent); err != nil {
		return SwapRoute{}, err
	}
	return route, nil
}

func (l routeAccounts) check(accounts []SolanaAddress, intent SwapIntent) error {
	const op = "chain.CheckSwap.route"
	inATAs, outATAs := associated(intent.Wallet, intent.In.Address), associated(intent.Wallet, intent.Out.Address)
	switch {
	case accounts[l.authority] != intent.Wallet:
		return refusal(
			op,
			"route spends from another authority",
			slog.String("authority", string(accounts[l.authority])),
		)
	case !slices.Contains(inATAs, accounts[l.source]):
		return refusal(op, "route spends from a token account that is not the treasury input account")
	case !slices.Contains(outATAs, accounts[l.destination]):
		return refusal(op, "route pays a token account that is not the treasury output account")
	case accounts[l.destinationMint] != intent.Out.Address:
		return refusal(
			op,
			"route output mint differs from the trade",
			slog.String("mint", string(accounts[l.destinationMint])),
		)
	case l.sourceMint >= 0 && accounts[l.sourceMint] != intent.In.Address:
		return refusal(
			op,
			"route input mint differs from the trade",
			slog.String("mint", string(accounts[l.sourceMint])),
		)
	case l.optionalDestination >= 0 && accounts[l.optionalDestination] != JupiterV6Program &&
		!slices.Contains(outATAs, accounts[l.optionalDestination]):
		return refusal(op, "route pays a second token account")
	}
	return nil
}

func associated(owner, mint SolanaAddress) []SolanaAddress {
	var out []SolanaAddress
	for _, program := range tokenPrograms() {
		if a, err := AssociatedTokenAccount(owner, mint, program); err == nil {
			out = append(out, a)
		}
	}
	return out
}

type budget struct {
	price, limit uint64
	limited      bool
	others       int
}

func (b *budget) apply(ix Instruction) error {
	const op = "chain.CheckSwap.fee"
	if ix.Program != ComputeBudgetProgram {
		b.others++
		return nil
	}
	if len(ix.Data) == 0 {
		return refusal(op, "compute budget instruction shape")
	}
	tag, args := ix.Data[0], ix.Data[1:]
	switch {
	case tag == budgetUnitLimit && len(args) == 4 && !b.limited:
		b.limit, b.limited = uint64(binary.LittleEndian.Uint32(args)), true
	case tag == budgetUnitPrice && len(args) == 8:
		b.price = binary.LittleEndian.Uint64(args)
	case (tag == budgetHeapFrame || tag == budgetAccountsLimit) && len(args) == 4:
	default:
		return refusal(op, "compute budget instruction outside the swap allowlist", slog.Int("tag", int(tag)))
	}
	return nil
}

func budgetOf(ixs []Instruction) (budget, error) {
	var b budget
	for _, ix := range ixs {
		if err := b.apply(ix); err != nil {
			return budget{}, err
		}
	}
	if !b.limited {
		b.limit = uint64(min(max(b.others*defaultUnitsPerIx, 0), maxComputeUnits))
	}
	b.limit = min(b.limit, maxComputeUnits)
	return b, nil
}

func checkFee(ixs []Instruction, intent SwapIntent) error {
	b, err := budgetOf(ixs)
	if err != nil {
		return err
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(b.price), new(big.Int).SetUint64(b.limit))
	fee.Add(fee, big.NewInt(microLamportsPerUnit-1)).Quo(fee, big.NewInt(microLamportsPerUnit))
	if fee.Cmp(new(big.Int).SetUint64(intent.MaxFeeLamports)) > 0 {
		return refusal("chain.CheckSwap.fee", "priority fee above the ceiling", slog.String("lamports", fee.String()))
	}
	return nil
}

func refusal(op, reason string, attrs ...slog.Attr) error {
	return errs.New(errs.CodeInvalidInput, op, append(attrs, slog.String("reason", reason))...)
}
