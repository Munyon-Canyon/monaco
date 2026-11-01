package relayer

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"log/slog"
	"math/bits"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	ComputeBudgetProgram chain.SolanaAddress = "ComputeBudget111111111111111111111111111111"
	JupiterV6Program     chain.SolanaAddress = "JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4"
	JupiterZProgram      chain.SolanaAddress = "61DFfeTKM7trxYcPQCM78bJ794ddZprZpAwAnLiwTpYH"
)

const (
	DefaultMaxFeeLamports       = 1_000_000
	DefaultMaxATACreations      = 2
	DefaultATARentLamports      = 2_500_000
	defaultComputeUnitLimit     = 1_400_000
	baseFeePerSignatureLamports = 5_000
	feePayerIndex               = 0
	ataFunderPosition           = 0
	ataMinAccounts              = 6
)

const (
	blockhashSize             = 32
	microLamportsPerLamport   = 1_000_000
	versionedMessagePrefixBit = 0x80
	budgetSetUnitLimit        = 2
	budgetSetUnitPrice        = 3
	budgetSetLoadedAccounts   = 4
	ataCreateIdempotent       = 1
	systemTransferTag         = 2
	systemTransferDataSize    = 12
	systemTransferAccounts    = 2
	transferRecipientPosition = 1
)

type Limits struct {
	MaxFeeLamports  uint64
	MaxATACreations uint64
	ATARentLamports uint64
	PayerPrograms   []chain.SolanaAddress
}

func DefaultLimits() Limits {
	return Limits{
		MaxFeeLamports:  DefaultMaxFeeLamports,
		MaxATACreations: DefaultMaxATACreations,
		ATARentLamports: DefaultATARentLamports,
		PayerPrograms:   []chain.SolanaAddress{JupiterV6Program},
	}
}

func (l Limits) MaxRentLamports() uint64 { return l.MaxATACreations * l.ATARentLamports }

type Spend struct {
	PriorityFeeLamports uint64
	BaseFeeLamports     uint64
	ATACreations        uint64
	RentLamports        uint64
}

func (s Spend) TotalLamports() uint64 {
	return s.PriorityFeeLamports + s.BaseFeeLamports + s.RentLamports
}

type swapInstruction struct {
	program  chain.SolanaAddress
	accounts []byte
	data     []byte
}

type message struct {
	signers      byte
	instructions []swapInstruction
}

type tally struct {
	unitLimit uint64
	unitPrice uint64
	atas      uint64
}

func (l Limits) inspect(msg []byte) (Spend, error) {
	const op = "relayer.inspect"
	refuse := func(reason string, attrs ...slog.Attr) (Spend, error) {
		all := append([]slog.Attr{slog.String("reason", reason)}, attrs...)
		return Spend{}, errs.New(errs.CodeInvalidInput, op, all...)
	}
	parsed, ok := parseMessage(msg)
	if !ok {
		return refuse("message structure")
	}
	t := tally{unitLimit: defaultComputeUnitLimit}
	for _, ix := range parsed.instructions {
		if reason := l.count(&t, ix); reason != "" {
			return refuse(reason, slog.String("program", string(ix.program)))
		}
	}
	priority, ok := priorityLamports(t.unitPrice, t.unitLimit)
	if !ok || priority > l.MaxFeeLamports {
		return refuse("priority fee over the cap", slog.Uint64("cap_lamports", l.MaxFeeLamports))
	}
	if t.atas > l.MaxATACreations {
		return refuse("relayer funds too many accounts", slog.Uint64("cap", l.MaxATACreations))
	}
	return Spend{
		PriorityFeeLamports: priority,
		BaseFeeLamports:     uint64(parsed.signers) * baseFeePerSignatureLamports,
		ATACreations:        t.atas,
		RentLamports:        t.atas * l.ATARentLamports,
	}, nil
}

func (l Limits) count(t *tally, ix swapInstruction) string {
	switch {
	case ix.program == ComputeBudgetProgram:
		limit, price, ok := budgetOf(ix.data)
		if !ok || len(ix.accounts) != 0 {
			return "compute budget instruction"
		}
		if limit != nil {
			t.unitLimit = *limit
		}
		if price != nil {
			t.unitPrice = *price
		}
	case !touchesFeePayer(ix), slices.Contains(l.PayerPrograms, ix.program), paysTheRelayer(ix):
	case isRelayerFundedATA(ix):
		t.atas++
	default:
		return "instruction takes the relayer key"
	}
	return ""
}

func touchesFeePayer(ix swapInstruction) bool {
	for _, a := range ix.accounts {
		if a == feePayerIndex {
			return true
		}
	}
	return false
}

func paysTheRelayer(ix swapInstruction) bool {
	return ix.program == chain.SystemProgram && len(ix.data) == systemTransferDataSize &&
		binary.LittleEndian.Uint32(ix.data) == systemTransferTag && len(ix.accounts) == systemTransferAccounts &&
		ix.accounts[ataFunderPosition] != feePayerIndex && ix.accounts[transferRecipientPosition] == feePayerIndex
}

func isRelayerFundedATA(ix swapInstruction) bool {
	if ix.program != chain.ATAProgram || len(ix.data) != 1 || ix.data[0] != ataCreateIdempotent {
		return false
	}
	if len(ix.accounts) < ataMinAccounts || ix.accounts[ataFunderPosition] != feePayerIndex {
		return false
	}
	for _, a := range ix.accounts[ataFunderPosition+1:] {
		if a == feePayerIndex {
			return false
		}
	}
	return true
}

func logSpend(ctx context.Context, s Spend, l Limits) {
	observability.Info(ctx, observability.RelayerCoSigned,
		slog.Uint64("priority_fee_lamports", s.PriorityFeeLamports),
		slog.Uint64("base_fee_lamports", s.BaseFeeLamports),
		slog.Uint64("ata_creations", s.ATACreations),
		slog.Uint64("rent_lamports", s.RentLamports),
		slog.Uint64("total_lamports", s.TotalLamports()),
		slog.Uint64("fee_cap_lamports", l.MaxFeeLamports),
		slog.Uint64("rent_cap_lamports", l.MaxRentLamports()),
	)
}

func priorityLamports(microLamportsPerUnit, units uint64) (uint64, bool) {
	hi, lo := bits.Mul64(microLamportsPerUnit, units)
	lo, carry := bits.Add64(lo, microLamportsPerLamport-1, 0)
	hi += carry
	if hi >= microLamportsPerLamport {
		return 0, false
	}
	lamports, _ := bits.Div64(hi, lo, microLamportsPerLamport)
	return lamports, true
}

func budgetOf(data []byte) (limit, price *uint64, ok bool) {
	if len(data) == 0 {
		return nil, nil, false
	}
	read := func(width int) (uint64, bool) {
		if len(data) != 1+width {
			return 0, false
		}
		var v uint64
		for i := width - 1; i >= 0; i-- {
			v = v<<8 | uint64(data[1+i])
		}
		return v, true
	}
	switch data[0] {
	case budgetSetUnitLimit:
		v, ok := read(4)
		return &v, nil, ok
	case budgetSetUnitPrice:
		v, ok := read(8)
		return nil, &v, ok
	case budgetSetLoadedAccounts:
		_, ok := read(4)
		return nil, nil, ok
	}
	return nil, nil, false
}

func parseMessage(msg []byte) (message, bool) {
	if len(msg) > 0 && msg[0]&versionedMessagePrefixBit != 0 {
		msg = msg[1:]
	}
	if len(msg) < 3 {
		return message{}, false
	}
	out := message{signers: msg[0]}
	keys, rest, ok := shortVec(msg[3:])
	if !ok || out.signers == 0 || int(out.signers) > keys || len(rest) < keys*ed25519.PublicKeySize+blockhashSize {
		return message{}, false
	}
	programs := make([]chain.SolanaAddress, keys)
	for i := range programs {
		programs[i] = chain.AddressOf(rest[i*ed25519.PublicKeySize : (i+1)*ed25519.PublicKeySize])
	}
	n, rest, ok := shortVec(rest[keys*ed25519.PublicKeySize+blockhashSize:])
	if !ok {
		return message{}, false
	}
	out.instructions = make([]swapInstruction, 0, n)
	for range n {
		var ix swapInstruction
		if ix, rest, ok = nextInstruction(programs, rest); !ok {
			return message{}, false
		}
		out.instructions = append(out.instructions, ix)
	}
	return out, true
}

func nextInstruction(programs []chain.SolanaAddress, b []byte) (ix swapInstruction, rest []byte, ok bool) {
	if len(b) < 1 || int(b[0]) >= len(programs) {
		return swapInstruction{}, nil, false
	}
	count, after, ok := shortVec(b[1:])
	if !ok || len(after) < count {
		return swapInstruction{}, nil, false
	}
	accounts := after[:count]
	size, after, ok := shortVec(after[count:])
	if !ok || len(after) < size {
		return swapInstruction{}, nil, false
	}
	return swapInstruction{program: programs[b[0]], accounts: accounts, data: after[:size]}, after[size:], true
}

func shortVec(b []byte) (n int, rest []byte, ok bool) {
	for i := range 3 {
		if i >= len(b) {
			return 0, nil, false
		}
		n |= int(b[i]&0x7f) << (7 * i)
		if b[i]&0x80 == 0 {
			return n, b[i+1:], true
		}
	}
	return 0, nil, false
}
